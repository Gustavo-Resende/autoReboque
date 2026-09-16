// Package database cuida da conexão com o Postgres e das migrations.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Conectar abre um pool de conexões e confirma que o banco responde.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("criar pool: %w", err)
	}

	ctxPing, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao banco: %w", err)
	}
	return pool, nil
}
