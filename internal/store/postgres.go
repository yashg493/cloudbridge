package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Config holds the parameters required to open a PostgreSQL connection pool.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	MaxConns int32 // maximum number of connections in the pool
	MinConns int32 // minimum number of idle connections to maintain
}

// DB wraps a pgxpool.Pool and exposes helpers used across repositories.
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// New creates and validates a PostgreSQL connection pool from the provided Config.
// It pings the database before returning to confirm connectivity.
func New(ctx context.Context, cfg Config, logger *zap.Logger) (*DB, error) {
	// TODO: build DSN string from cfg fields
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)

	// TODO: parse into pgxpool.Config and set MaxConns, MinConns,
	//       MaxConnLifetime, MaxConnIdleTime, HealthCheckPeriod
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse pool config: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns

	// TODO: pgxpool.NewWithConfig(ctx, poolCfg)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("store: create pool: %w", err)
	}

	db := &DB{pool: pool, logger: logger}

	// Verify connectivity at startup.
	if err := db.HealthCheck(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: initial health check failed: %w", err)
	}

	logger.Info("postgres connection pool ready",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("db", cfg.DBName),
		zap.Int32("max_conns", cfg.MaxConns),
	)
	return db, nil
}

// Pool returns the underlying pgxpool for use in repositories.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// Close gracefully closes all connections in the pool.
func (d *DB) Close() {
	d.pool.Close()
	d.logger.Info("postgres connection pool closed")
}

// HealthCheck pings the database and returns an error if unavailable.
func (d *DB) HealthCheck(ctx context.Context) error {
	if err := d.pool.Ping(ctx); err != nil {
		return fmt.Errorf("store: ping failed: %w", err)
	}
	return nil
}
