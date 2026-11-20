package httpapi

import (
	"context"
	"net/http"
	"runtime"
	"time"
)

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	db := s.store.Pool().Ping(ctx) == nil
	redis := "disabled"
	if s.redis != nil {
		redis = "ok"
		if s.redis.Ping(ctx) != nil {
			redis = "error"
		}
	}
	queues, err := s.store.QueueDepth(ctx)
	if err != nil {
		queues = nil
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	total := s.m.total.Load()
	mean := float64(0)
	if total > 0 {
		mean = float64(s.m.durationNS.Load()) / float64(total) / 1e6
	}
	var backup map[string]float64
	if db && ctx.Err() == nil {
		backup = s.store.BackupStatus(ctx)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"at": time.Now().UTC(), "started_at": s.m.started.UTC(), "uptime_seconds": time.Since(s.m.started).Seconds(),
		"database_ok": db, "redis": redis, "heap_bytes": memory.HeapAlloc, "runtime_bytes": memory.Sys,
		"goroutines": runtime.NumGoroutine(), "websockets": s.hub.Connections(),
		"requests": total, "errors_5xx": s.m.errors5xx.Load(), "mean_latency_ms": mean,
		"pool": s.store.PoolStats(), "queues": queues, "backup": backup,
	})
}
