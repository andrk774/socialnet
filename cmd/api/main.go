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
	"socialnet/internal/config"
	"socialnet/internal/db"
	"socialnet/internal/httpapi"
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

	// Фоновый обработчик видео: `api worker` (отдельный контейнер).
	if len(os.Args) == 2 && os.Args[1] == "worker" {
		avStore, err := av.NewStore(filepath.Join(cfg.MediaDir, "av"))
		if err != nil {
			return err
		}
		(&worker.Worker{Store: store.New(pool), AV: avStore, Log: log}).Run(ctx)
		return nil
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

	// Callback API: доставка событий серверам сообществ.
	go api.RunCallbacks(ctx)

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
