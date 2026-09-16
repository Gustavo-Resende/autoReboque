package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sessoes cria e valida sessões de login.
//
// O cookie leva um token aleatório; no banco fica só o SHA-256 dele.
// Assim, quem ler a tabela não consegue forjar um cookie.
type Sessoes struct {
	pool    *pgxpool.Pool
	duracao time.Duration
}

func NovasSessoes(pool *pgxpool.Pool, duracao time.Duration) *Sessoes {
	return &Sessoes{pool: pool, duracao: duracao}
}

// Duracao é o tempo de vida de uma sessão (usado no MaxAge do cookie).
func (s *Sessoes) Duracao() time.Duration { return s.duracao }

// Criar gera um token novo, grava a sessão e devolve o token para o cookie.
// Aproveita para apagar sessões expiradas — é a "limpeza" da tabela.
func (s *Sessoes) Criar(ctx context.Context) (string, error) {
	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		return "", fmt.Errorf("gerar token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(bruto)

	if _, err := s.pool.Exec(ctx, `DELETE FROM sessao WHERE expira_em < now()`); err != nil {
		return "", fmt.Errorf("limpar sessões: %w", err)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessao (token_hash, expira_em) VALUES ($1, $2)`,
		hashToken(token), time.Now().Add(s.duracao))
	if err != nil {
		return "", fmt.Errorf("gravar sessão: %w", err)
	}
	return token, nil
}

// Valida diz se o token corresponde a uma sessão viva.
func (s *Sessoes) Valida(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var existe bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM sessao WHERE token_hash = $1 AND expira_em > now())`,
		hashToken(token)).Scan(&existe)
	return existe, err
}

// Encerrar apaga a sessão (logout).
func (s *Sessoes) Encerrar(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessao WHERE token_hash = $1`, hashToken(token))
	return err
}

func hashToken(token string) string {
	soma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(soma[:])
}
