package db

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/logger"
)

func NewPool(DBHost, DBName, DBUser, DBPass string) *pgxpool.Pool {
	encodedPassword := url.QueryEscape(DBPass)
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s/%s",
		DBUser,
		encodedPassword,
		DBHost,
		DBName,
	)
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		logger.Fatalf("Failed to parse DB config: %v", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.MaxConns = 10
	cfg.MinConns = 5
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.MaxConnLifetime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		logger.Fatalf("DB connection failed: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		logger.Fatalf("DB ping failed: %v", err)
	}
	return pool
}
