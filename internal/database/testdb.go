package database

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolDeTeste conecta ao banco apontado por TEST_DATABASE_URL, aplica as
// migrations e limpa todas as tabelas. Se a variável não estiver definida,
// o teste é pulado: assim `go test ./...` funciona sem Postgres, e os testes
// de integração rodam quando o banco de teste está disponível.
//
// Nunca aponte TEST_DATABASE_URL para o banco de uso real: os dados são apagados.
func PoolDeTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definida; pulando teste de integração")
	}
	ctx := context.Background()
	pool, err := Conectar(ctx, url)
	if err != nil {
		t.Fatalf("conectar ao banco de teste: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := Migrar(ctx, pool); err != nil {
		t.Fatalf("migrar banco de teste: %v", err)
	}
	// Clientes e serviços de sistema (semeados na migration) ficam; o resto vai.
	_, err = pool.Exec(ctx, `
		TRUNCATE ordem_servico, sequencia_documento, sessao, motorista RESTART IDENTITY CASCADE;
		DELETE FROM cliente WHERE sistema = false;
		DELETE FROM servico WHERE id > 7;`)
	if err != nil {
		t.Fatalf("limpar banco de teste: %v", err)
	}
	return pool
}
