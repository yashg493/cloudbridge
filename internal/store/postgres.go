package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by repository methods when a requested record does not exist.
// Callers should use errors.Is to check for this sentinel.
var ErrNotFound = errors.New("store: record not found")

const (
	poolMaxConns          = int32(20)
	poolMinConns          = int32(5)
	poolMaxConnLifetime   = time.Hour
	poolHealthCheckPeriod = 30 * time.Second
)

// NewPool creates a pgxpool.Pool connected to databaseURL and validates connectivity
// with a ping before returning. All pool settings are fixed at production-grade defaults:
//
//	MaxConns:          20
//	MinConns:           5
//	MaxConnLifetime:    1 hour
//	HealthCheckPeriod: 30 seconds
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store.NewPool: parse URL: %w", err)
	}

	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.HealthCheckPeriod = poolHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store.NewPool: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store.NewPool: ping database: %w", err)
	}

	return pool, nil
}
