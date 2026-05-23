package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/api"
	"github.com/yashg493/cloudbridge/internal/metrics"
	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

func main() {
	// zap.NewProduction emits JSON logs; swap for zap.NewDevelopment during local debugging.
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// signal.NotifyContext cancels ctx on SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Fatal("server exited with error", zap.Error(err))
	}
}

// run contains all startup and shutdown logic so it is testable without os.Exit.
func run(ctx context.Context, logger *zap.Logger) error {
	// ── Config from environment ──────────────────────────────────────────────
	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	dbCfg := store.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     dbPort,
		User:     getEnv("DB_USER", "cloudbridge"),
		Password: getEnv("DB_PASSWORD", "cloudbridge"),
		DBName:   getEnv("DB_NAME", "cloudbridge"),
		MaxConns: 20,
		MinConns: 2,
	}

	workerCount, _ := strconv.Atoi(getEnv("WORKER_COUNT", "10"))
	workerQueueSize, _ := strconv.Atoi(getEnv("WORKER_QUEUE_SIZE", "100"))

	// ── Database ─────────────────────────────────────────────────────────────
	// TODO: uncomment when Postgres is available in the target environment.
	// db, err := store.New(ctx, dbCfg, logger)
	// if err != nil {
	//     return fmt.Errorf("failed to connect to postgres: %w", err)
	// }
	// defer db.Close()
	var db *store.DB // placeholder until DB is wired
	_ = dbCfg
	logger.Warn("database not yet initialised — set DB_* env vars and uncomment store.New in main.go")

	// ── Repositories ─────────────────────────────────────────────────────────
	fileRepo := store.NewFileRepository(db, logger)
	nsRepo := store.NewNamespaceRepository(db, logger)

	// ── Cloud provider ───────────────────────────────────────────────────────
	// TODO: select provider from CLOUD_PROVIDER env var (s3 | gcs)
	// TODO: cloud.NewS3Provider(ctx, s3Cfg, logger) or cloud.NewGCSProvider(...)
	// var provider cloud.Provider

	// ── Metrics ──────────────────────────────────────────────────────────────
	metricsReg := metrics.NewRegistry()
	_ = metricsReg // injected into router when middleware is wired

	// ── Worker pool ──────────────────────────────────────────────────────────
	pool := worker.NewPool(ctx, workerCount, workerQueueSize, logger)
	pool.Start()
	defer pool.Stop()

	// ── Tiering scheduler ────────────────────────────────────────────────────
	// TODO: uncomment once provider and fileRepo are fully implemented.
	// scheduler := worker.NewScheduler(pool, fileRepo, provider, logger, 5*time.Minute)
	// go scheduler.Run(ctx)

	// ── HTTP server ──────────────────────────────────────────────────────────
	router := api.NewRouter(api.RouterConfig{
		Logger:     logger,
		DB:         db,
		FileRepo:   fileRepo,
		NSRepo:     nsRepo,
		WorkerPool: pool,
		MetricsReg: metricsReg,
	})

	srv := &http.Server{
		Addr:         ":" + getEnv("HTTP_PORT", "8080"),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in the background.
	srvErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
	}()

	// Block until shutdown signal or server error.
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining…")
	case err := <-srvErr:
		return fmt.Errorf("http server error: %w", err)
	}

	// Graceful shutdown: give in-flight requests up to 30 s to finish.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}

// getEnv returns the value of the environment variable key, or defaultVal if unset.
func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return defaultVal
}
