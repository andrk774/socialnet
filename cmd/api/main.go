package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"socialnet/internal/av"
	"socialnet/internal/backup"
	"socialnet/internal/config"
	"socialnet/internal/db"
	"socialnet/internal/httpapi"
	"socialnet/internal/realtime"
	"socialnet/internal/redisx"
	"socialnet/internal/store"
	"socialnet/internal/webpush"
	"socialnet/internal/worker"

	_ "time/tzdata" // часовые пояса для тихого режима: в минимальном образе их нет
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// Ключи для Web Push: `api gen-vapid`, вывод положить в .env
	if len(os.Args) == 2 && os.Args[1] == "gen-vapid" {
		pub, priv, err := webpush.GenerateVAPID()
		if err != nil {
			return err
		}
		fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", pub, priv)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 30*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Info("migrations applied")

	// Назначить первого администратора:
	//   docker compose exec api /api make-admin user@example.com
	if len(os.Args) == 3 && os.Args[1] == "make-admin" {
		tag, err := pool.Exec(ctx, `UPDATE users SET role = 'admin' WHERE email = $1`, strings.ToLower(os.Args[2]))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("user not found: " + os.Args[2])
		}
		log.Info("user is now admin", "email", os.Args[2])
		return nil
	}

	// Резервные копии: `api backup` — один раз, `api backup-loop` — каждый день
	// в BACKUP_HOUR (UTC), `api restore <файл> --yes` — восстановить (затирает данные!).
	if len(os.Args) >= 2 && (os.Args[1] == "backup" || os.Args[1] == "backup-loop") {
		return runBackups(ctx, cfg, store.New(pool), log, os.Args[1] == "backup-loop")
	}
	if len(os.Args) >= 3 && os.Args[1] == "restore" {
		if len(os.Args) < 4 || os.Args[3] != "--yes" {
			return errors.New("restore replaces all data in DATABASE_URL; add --yes to confirm")
		}
		if err := backup.Restore(ctx, os.Args[2], cfg.DatabaseURL, cfg.BackupKey); err != nil {
			return err
		}
		log.Info("restored", "file", os.Args[2])
		return nil
	}

	// Фоновый обработчик видео: `api worker` (отдельный контейнер).
	if len(os.Args) == 2 && os.Args[1] == "worker" {
		avStore, err := av.NewStore(filepath.Join(cfg.MediaDir, "av"))
		if err != nil {
			return err
		}
		avStore.Remote = httpapi.NewS3(cfg)
		wk := &worker.Worker{Store: store.New(pool), AV: avStore, Log: log}
		if cfg.RedisURL != "" {
			opt, err := redisx.ParseURL(cfg.RedisURL)
			if err != nil {
				return err
			}
			rc := redisx.New(opt)
			defer rc.Close()
			wk.Events = func(ctx context.Context, user int64, typ string, data any) error {
				msg, err := realtime.Encode("worker", user, typ, data)
				if err != nil {
					return err
				}
				_, err = rc.Do(ctx, "PUBLISH", realtime.BusChannel, msg)
				return err
			}
		}
		wk.Run(ctx)
		return nil
	}

	if s3c := httpapi.NewS3(cfg); s3c != nil {
		if err := s3c.EnsureBucket(ctx); err != nil {
			log.Warn("s3 bucket check", "err", err)
		}
	}
	api, err := httpapi.New(cfg, store.New(pool), log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.Router(),
		// Защита от slowloris: на заголовки 5 секунд. Таймауты чтения тела
		// и записи заданы для каждого маршрута отдельно (см. httpapi.deadlines).
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	// Раз в 15 минут — письма тем, кто пропустил уведомления.
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := api.RunDigest(ctx); err != nil {
					log.Error("digest", "err", err)
				} else if n > 0 {
					log.Info("digest emails sent", "count", n)
				}
			}
		}
	}()

	// Шина событий (Redis), доставка Callback API, детектор аномалий.
	api.Start(ctx)

	// /metrics — на отдельном адресе, который не публикуется наружу.
	if cfg.MetricsAddr != "off" {
		msrv := &http.Server{Addr: cfg.MetricsAddr, Handler: api.MetricsHandler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := msrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.Error("metrics server", "err", err)
			}
		}()
		defer msrv.Close() //nolint:errcheck
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shCtx)
}

func runBackups(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger, loop bool) error {
	o := backup.Options{DatabaseURL: cfg.DatabaseURL, Dir: cfg.BackupDir, Key: cfg.BackupKey, Keep: cfg.BackupKeep,
		S3Prefix: "backups/", Log: log}
	if cfg.BackupS3 {
		o.S3 = httpapi.NewS3(cfg)
	}
	if o.Key == nil {
		log.Warn("BACKUP_ENC_KEY is not set: backups are stored unencrypted")
	}
	if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
		return err
	}
	once := func() error {
		res, err := backup.Run(ctx, o)
		if res != nil {
			if rerr := st.RecordBackup(ctx, res.Name, res.Size, res.SHA256, res.Encrypted, res.RestoreOK,
				res.RestoreErr, res.Duration); rerr != nil {
				log.Error("record backup", "err", rerr)
			}
		} else if err != nil {
			_ = st.RecordBackup(ctx, "", 0, "", o.Key != nil, false, err.Error(), 0)
		}
		if err != nil {
			log.Error("backup failed", "err", err)
			return err
		}
		log.Info("backup ok", "name", res.Name, "size", res.Size, "tables", res.Tables, "duration", res.Duration.String())
		return nil
	}
	if !loop {
		return once()
	}
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), cfg.BackupHour, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		log.Info("next backup", "at", next)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Until(next)):
		}
		_ = once() // ошибка уже в логе и в метрике; следующая попытка — завтра
	}
}
