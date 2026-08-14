// Command worker runs the ProGuideGH background job runner (Phase 0:
// config/DB/Redis wiring, health endpoint, and a ticking job-loop skeleton
// with no real jobs yet).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"proguidegh/worker/internal/platform/config"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := connectPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := connectRedis(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("connect redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close() //nolint:errcheck

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WorkerPort),
		Handler:           healthzHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("worker health listening", "port", cfg.WorkerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("health server", "error", err)
		}
	}()

	go runJobLoop(ctx, log, cfg.TickInterval)

	log.Info("worker started", "env", cfg.AppEnv, "tick", cfg.TickInterval.String())
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	log.Info("worker stopped")
}

// healthzHandler exposes liveness for the worker process on WORKER_PORT.
func healthzHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

// runJobLoop ticks on the configured interval. Phase 0 only logs each tick;
// real jobs (emails, push, PDF, webhook retries, payout batches, scheduled
// quality/expiry checks — spec §21) are registered here in later phases.
func runJobLoop(ctx context.Context, log *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			log.Info("job loop tick", "at", t.UTC().Format(time.RFC3339), "jobs", 0)
		}
	}
}

func connectPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

func connectRedis(ctx context.Context, redisURL string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	client := goredis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return client, nil
}
