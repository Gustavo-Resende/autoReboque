package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// As migrations ficam embutidas no binário: em produção não é preciso
// levar a pasta junto, e o esquema do banco anda sempre com o código.
//
//go:embed migrations/*.sql
var arquivosMigrations embed.FS

// migration é um arquivo SQL numerado, ex.: 001_schema.sql.
type migration struct {
	versao int
	nome   string
	sql    string
}

// chaveLockMigracao identifica o advisory lock que serializa migrações
// concorrentes (duas instâncias subindo juntas, ou pacotes de teste em paralelo).
const chaveLockMigracao = 8731_0001

// Migrar aplica, em ordem, todas as migrations que ainda não foram aplicadas.
// Cada uma roda dentro de uma transação: ou entra inteira ou não entra.
// Só existe "up"; para desfazer algo, escreve-se uma nova migration.
func Migrar(ctx context.Context, pool *pgxpool.Pool) error {
	// O lock é por sessão, então tudo precisa acontecer na mesma conexão.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("obter conexão para migrar: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, chaveLockMigracao); err != nil {
		return fmt.Errorf("obter lock de migração: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, chaveLockMigracao)

	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			versao      integer     PRIMARY KEY,
			nome        text        NOT NULL,
			aplicada_em timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}

	aplicadas := map[int]bool{}
	linhas, err := conn.Query(ctx, `SELECT versao FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("ler schema_migrations: %w", err)
	}
	for linhas.Next() {
		var v int
		if err := linhas.Scan(&v); err != nil {
			linhas.Close()
			return err
		}
		aplicadas[v] = true
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return err
	}

	pendentes, err := lerMigrations()
	if err != nil {
		return err
	}

	for _, m := range pendentes {
		if aplicadas[m.versao] {
			continue
		}
		if err := aplicar(ctx, conn, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.nome, err)
		}
		slog.Info("migration aplicada", "arquivo", m.nome)
	}
	return nil
}

func aplicar(ctx context.Context, conn *pgxpool.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback depois de Commit é inofensivo; garante limpeza em caso de erro.
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (versao, nome) VALUES ($1, $2)`, m.versao, m.nome); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// lerMigrations lista os arquivos embutidos ordenados pela versão numérica
// do prefixo do nome (001_, 002_, ...).
func lerMigrations() ([]migration, error) {
	entradas, err := fs.ReadDir(arquivosMigrations, "migrations")
	if err != nil {
		return nil, err
	}
	var lista []migration
	for _, e := range entradas {
		nome := e.Name()
		prefixo, _, ok := strings.Cut(nome, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q: nome deve ser NNN_descricao.sql", nome)
		}
		versao, err := strconv.Atoi(prefixo)
		if err != nil {
			return nil, fmt.Errorf("migration %q: prefixo numérico inválido", nome)
		}
		conteudo, err := arquivosMigrations.ReadFile("migrations/" + nome)
		if err != nil {
			return nil, err
		}
		lista = append(lista, migration{versao: versao, nome: nome, sql: string(conteudo)})
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].versao < lista[j].versao })
	for i := 1; i < len(lista); i++ {
		if lista[i].versao == lista[i-1].versao {
			return nil, fmt.Errorf("migrations %q e %q têm a mesma versão", lista[i-1].nome, lista[i].nome)
		}
	}
	return lista, nil
}
