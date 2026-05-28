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

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/api"
	"github.com/yashg493/cloudbridge/internal/cloud"
	"github.com/yashg493/cloudbridge/internal/metrics"
	"github.com/yashg493/cloudbridge/internal/nfs"
	"github.com/yashg493/cloudbridge/internal/store"
	"github.com/yashg493/cloudbridge/internal/worker"
)

func main() {
	logLvl := os.Getenv("LOG_LEVEL")
	logger, err := buildLogger(logLvl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Fatal("server exited with error", zap.Error(err))
	}
}

// run contains all startup and shutdown logic so it is testable without os.Exit.
func run(ctx context.Context, logger *zap.Logger) error {
	// ── Config ──────────────────────────────────────────────────────────────────
	port := getEnv("PORT", "8080")
	databaseURL := getEnv("DATABASE_URL",
		"postgres://postgres:postgres@localhost:5432/cloudbridge")
	workerCount, _ := strconv.Atoi(getEnv("WORKER_COUNT", "10"))
	cloudBackend := getEnv("CLOUD_BACKEND", "mock")
	nfsBasePath := getEnv("NFS_BASE_PATH", "")
	migrationsPath := getEnv("MIGRATIONS_PATH", "migrations/001_init.sql")

	logger.Info("CloudBridge starting",
		zap.String("port", port),
		zap.String("cloud_backend", cloudBackend),
		zap.String("nfs_base_path", nfsBasePath),
		zap.Int("worker_count", workerCount),
	)

	// ── PostgreSQL (5 attempts, 2 s backoff) ──────────────────────────────────
	pool, err := connectDB(ctx, databaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	// ── DB migrations ─────────────────────────────────────────────────────────
	if err := runMigrations(ctx, pool, migrationsPath, logger); err != nil {
		logger.Warn("migration failed — schema may already be up-to-date",
			zap.String("path", migrationsPath), zap.Error(err))
	}

	// ── Repositories ─────────────────────────────────────────────────────────
	fileRepo := store.NewFileRepo(pool)
	nsRepo := store.NewNamespaceRepo(pool)
	syncJobRepo := store.NewSyncJobRepo(pool)

	// ── NFS simulator ───────────────────────────────────────────────────────────
	nfsSim, err := nfs.New(nfsBasePath, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise NFS simulator: %w", err)
	}

	// ── Cloud provider ───────────────────────────────────────────────────────────
	cloudProvider := selectCloudProvider(ctx, cloudBackend, logger)

	// ── Metrics ────────────────────────────────────────────────────────────────
	metricsReg := metrics.NewRegistry()

	// ── Worker pool ─────────────────────────────────────────────────────────────
	// Declare defer first (LIFO): pool closes after workers drain
	workerPool := worker.NewWorkerPool(ctx, workerCount, worker.Deps{
		FileRepo:    fileRepo,
		NSRepo:      nsRepo,
		SyncJobRepo: syncJobRepo,
		Provider:    cloudProvider,
		Metrics:     metricsReg,
		Logger:      logger,
	})
	workerPool.Start()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := workerPool.Shutdown(shutCtx); err != nil {
			logger.Error("worker pool shutdown", zap.Error(err))
		}
	}()

	// ── Job scheduler ───────────────────────────────────────────────────────────
	scheduler := worker.NewScheduler(workerPool, syncJobRepo, logger)
	scheduler.Start()
	defer scheduler.Stop()

	// ── HTTP server ─────────────────────────────────────────────────────────────
	router := api.NewRouter(api.RouterConfig{
		Logger:      logger,
		Pool:        pool,
		FileRepo:    fileRepo,
		NSRepo:      nsRepo,
		SyncJobRepo: syncJobRepo,
		WorkerPool:  workerPool,
		NFSSim:      nfsSim,
		MetricsReg:  metricsReg,
	})
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("CloudBridge gateway started", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
	}()

	// ── Block until SIGTERM/SIGINT or server error ──────────────────────────────
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-srvErr:
		return fmt.Errorf("http server error: %w", err)
	}

	// ── Graceful shutdown (order: HTTP → scheduler → workers → DB) ───────────────
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warn("HTTP server shutdown", zap.Error(err))
	}
	// scheduler, workerPool, and pool.Close() are handled by defers above.

	logger.Info("CloudBridge stopped cleanly")
	return nil
}

// buildLogger creates a zap.Logger based on the LOG_LEVEL env var.
//   - "development" / "dev"  → coloured console output (for local use)
//   - "debug"               → production JSON at Debug level
//   - "warn"                → production JSON at Warn level
//   - ""  / "info" (default)→ production JSON at Info level
func buildLogger(level string) (*zap.Logger, error) {
	switch level {
	case "development", "dev":
		return zap.NewDevelopment()
	default:
		cfg := zap.NewProductionConfig()
		switch level {
		case "debug":
			cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		case "warn", "warning":
			cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
		case "error":
			cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		}
		return cfg.Build()
	}
}

// connectDB opens a pgxpool and retries up to 5 times with a 2-second backoff.
func connectDB(ctx context.Context, databaseURL string, logger *zap.Logger) (*pgxpool.Pool, error) {
	const maxAttempts = 5
	const retryBackoff = 2 * time.Second

	var (
		p   *pgxpool.Pool
		err error
	)
	for i := range maxAttempts {
		p, err = store.NewPool(ctx, databaseURL)
		if err == nil {
			logger.Info("connected to PostgreSQL",
				zap.String("url", redactURL(databaseURL)))
			return p, nil
		}
		if i < maxAttempts-1 {
			logger.Warn("DB connection failed, retrying",
				zap.Int("attempt", i+1),
				zap.Duration("backoff", retryBackoff),
				zap.Error(err))
			select {
			case <-time.After(retryBackoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("failed to connect to PostgreSQL after %d attempts: %w", maxAttempts, err)
}

// runMigrations reads and executes a SQL migration file.
// The migration uses CREATE TABLE IF NOT EXISTS so it is safe to run on every startup.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, path string, logger *zap.Logger) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", path, err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("execute migration %q: %w", path, err)
	}
	logger.Info("migrations applied", zap.String("path", path))
	return nil
}

// selectCloudProvider returns the cloud backend selected by CLOUD_BACKEND env var.
func selectCloudProvider(ctx context.Context, backend string, logger *zap.Logger) cloud.CloudProvider {
	switch backend {
	case "s3", "aws":
		logger.Info("cloud backend: S3 (reading AWS_REGION + AWS_BUCKET)")
		return cloud.NewProvider(ctx, logger) // falls back to mock if env vars missing
	case "gcs", "google":
		logger.Warn("cloud backend: GCS stub — all ops return ErrNotImplemented")
		return &cloud.GCSProvider{}
	default: // "mock" or anything else
		logger.Info("cloud backend: in-memory mock (no persistence)")
		return cloud.NewMockS3Provider(logger)
	}
}

// redactURL removes the password from a postgres:// URL for safe logging.
func redactURL(u string) string {
	for i, c := range u {
		if c == '@' {
			return "postgres://***@" + u[i+1:]
		}
	}
	return u
}

// getEnv returns the value of the environment variable key, or defaultVal if unset.
func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return defaultVal
}
