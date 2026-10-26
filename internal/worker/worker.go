// Package worker: фоновая обработка задач из очереди (перекодирование видео).
//
// Воркер запускается отдельным процессом (`api worker`) и в Docker
// живёт в изолированной сети без доступа в интернет: если в ffmpeg найдётся
// уязвимость, злоумышленник окажется в контейнере, откуда некуда идти.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialnet/internal/av"
	"socialnet/internal/store"
)

const (
	MaxVideoSeconds = 30 * 60
)

type Worker struct {
	Store *store.Store
	AV    *av.Store
	Log   *slog.Logger
	// Events — мгновенное событие пользователю через шину API (Redis).
	// nil — пользователь узнает о готовности из уведомления и опроса статуса.
	Events func(ctx context.Context, userID int64, typ string, data any) error
}

// Run обрабатывает задачи, пока не отменят ctx.
func (w *Worker) Run(ctx context.Context) {
	w.Log.Info("worker started")
	lastStale := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastStale) > 5*time.Minute {
			if n, err := w.Store.RequeueStale(ctx, time.Hour); err == nil && n > 0 {
				w.Log.Warn("requeued stale jobs", "count", n)
			}
			lastStale = time.Now()
		}
		did, err := w.ProcessOne(ctx)
		if err != nil {
			w.Log.Error("job", "err", err)
		}
		if !did {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// ProcessOne берёт одну задачу и выполняет её. false — очередь пуста.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	job, err := w.Store.ClaimJob(ctx)
	if err != nil || job == nil {
		return false, err
	}
	switch job.Kind {
	case "video_transcode":
		err = w.transcodeVideo(ctx, job)
	case "video_mp4":
		err = w.backfillMP4(ctx, job)
	default:
		err = errors.New("unknown job kind " + job.Kind)
	}
	if err == nil {
		return true, w.Store.CompleteJob(ctx, job.ID)
	}
	w.Log.Warn("job failed", "id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "err", err)
	final, ferr := w.Store.FailJob(ctx, job.ID, err.Error(), time.Duration(job.Attempts)*time.Minute)
	if ferr != nil {
		return true, ferr
	}
	if final && job.Kind == "video_transcode" {
		_ = w.Store.VideoFailed(ctx, job.Payload["video_id"], "processing failed")
		if src, err := w.AV.SourcePath(job.Payload["video_id"]); err == nil {
			_ = os.Remove(src)
		}
	}
	return true, nil
}

func (w *Worker) transcodeVideo(ctx context.Context, job *store.Job) error {
	id, format := job.Payload["video_id"], job.Payload["format"]
	if format != "mov" && format != "matroska" {
		return errors.New("bad format in job")
	}
	src, err := w.AV.SourcePath(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		_ = w.Store.VideoFailed(ctx, id, "source missing")
		return nil // повторять бессмысленно
	}
	probe, err := av.ProbeFile(ctx, src)
	if err != nil || probe.Video() == nil {
		_ = w.Store.VideoFailed(ctx, id, "cannot read video")
		_ = os.Remove(src)
		return nil
	}
	dir, err := w.AV.Dir("videos", id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	v := probe.Video()
	made, err := av.TranscodeHLS(ctx, src, probe.Format, dir, v.Height, MaxVideoSeconds, probe.Audio() != nil)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	poster, _ := w.AV.Path("videos", id, "poster.jpg")
	if err := av.Poster(ctx, src, probe.Format, poster, probe.Seconds); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// MP4 лучшего качества — для браузеров без HLS. Не получилось — видео
	// всё равно публикуем: HLS-версия есть.
	mp4 := made[len(made)-1]
	if err := av.RemuxMP4(ctx, dir, mp4); err != nil {
		w.Log.Warn("remux mp4", "id", id, "err", err)
		mp4 = ""
	}
	if err := w.AV.Commit(ctx, "videos", id); err != nil { // в S3, если настроено
		return err
	}
	secs := int(math.Min(math.Round(probe.Seconds), MaxVideoSeconds))
	owner, err := w.Store.VideoReady(ctx, id, secs, v.Width, v.Height, made, mp4)
	if errors.Is(err, store.ErrNotFound) {
		_ = w.AV.Delete("videos", id) // видео удалили, пока шла обработка
	} else if err != nil {
		return err
	} else {
		_, _ = w.Store.CreateNotification(ctx, store.NewNotification{UserID: owner, Type: "video_ready"})
		if w.Events != nil {
			if err := w.Events(ctx, owner, "video_ready", map[string]any{"id": id, "qualities": made}); err != nil {
				w.Log.Warn("publish video_ready", "err", err)
			}
		}
	}
	_ = os.Remove(src)
	w.Log.Info("video ready", "id", id, "qualities", made)
	return nil
}

// backfillMP4 собирает MP4 для видео, обработанного до появления MP4:
// скачивает плейлист и сегменты (с диска или из S3) и склеивает их.
func (w *Worker) backfillMP4(ctx context.Context, job *store.Job) error {
	id, rendition := job.Payload["video_id"], job.Payload["rendition"]
	tmpRoot, err := w.AV.TempDir()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(tmpRoot, "mp4-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	playlist := rendition + ".m3u8"
	if err := w.AV.Fetch(ctx, "videos", id, playlist, filepath.Join(tmp, playlist)); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(tmp, playlist)) //nolint:gosec // наш временный файл
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		// в плейлисте только наши имена сегментов; всё прочее — повод остановиться
		if _, err := w.AV.Key("videos", id, name); err != nil || !strings.HasSuffix(name, ".ts") {
			return errors.New("unexpected entry in playlist: " + name)
		}
		if err := w.AV.Fetch(ctx, "videos", id, name, filepath.Join(tmp, name)); err != nil {
			return err
		}
	}
	if err := av.RemuxMP4(ctx, tmp, rendition); err != nil {
		return err
	}
	if err := w.AV.PutFile(ctx, "videos", id, rendition+".mp4", tmp); err != nil {
		return err
	}
	return w.Store.SetVideoMP4(ctx, id, rendition)
}
