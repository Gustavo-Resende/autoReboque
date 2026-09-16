// Package motorista é a lista de quem dirige o guincho. Só nome e telefone:
// serve para escolher na OS em vez de digitar o nome de três jeitos.
package motorista

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNaoEncontrado   = errors.New("motorista não encontrado")
	ErrNomeObrigatorio = errors.New("informe o nome do motorista")
)

type Motorista struct {
	ID       int64
	Nome     string
	Telefone string
	Ativo    bool
	CriadoEm time.Time
}

func (m *Motorista) Validar() error {
	m.Nome = strings.TrimSpace(m.Nome)
	m.Telefone = strings.TrimSpace(m.Telefone)
	if m.Nome == "" {
		return ErrNomeObrigatorio
	}
	return nil
}

type Repositorio struct {
	pool      *pgxpool.Pool
	empresaID int64
}

func NovoRepositorio(pool *pgxpool.Pool, empresaID int64) *Repositorio {
	return &Repositorio{pool: pool, empresaID: empresaID}
}

const colunas = `id, nome, telefone, ativo, criado_em`

func ler(linha pgx.Row) (*Motorista, error) {
	var m Motorista
	err := linha.Scan(&m.ID, &m.Nome, &m.Telefone, &m.Ativo, &m.CriadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrado
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repositorio) Buscar(ctx context.Context, id int64) (*Motorista, error) {
	return ler(r.pool.QueryRow(ctx, `SELECT `+colunas+` FROM motorista WHERE id = $1 AND empresa_id = $2`, id, r.empresaID))
}

func (r *Repositorio) Listar(ctx context.Context, somenteAtivos bool) ([]Motorista, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT `+colunas+` FROM motorista WHERE empresa_id = $1 AND ($2 = false OR ativo = true) ORDER BY ativo DESC, lower(nome)`,
		r.empresaID, somenteAtivos)
	if err != nil {
		return nil, fmt.Errorf("listar motoristas: %w", err)
	}
	defer linhas.Close()
	var lista []Motorista
	for linhas.Next() {
		m, err := ler(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, *m)
	}
	return lista, linhas.Err()
}

func (r *Repositorio) Criar(ctx context.Context, m *Motorista) error {
	if err := m.Validar(); err != nil {
		return err
	}
	m.Ativo = true
	err := r.pool.QueryRow(ctx,
		`INSERT INTO motorista (empresa_id, nome, telefone) VALUES ($1, $2, $3) RETURNING id, criado_em`,
		r.empresaID, m.Nome, m.Telefone).Scan(&m.ID, &m.CriadoEm)
	if err != nil {
		return fmt.Errorf("criar motorista: %w", err)
	}
	return nil
}

func (r *Repositorio) Atualizar(ctx context.Context, m *Motorista) error {
	if err := m.Validar(); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE motorista SET nome = $3, telefone = $4, ativo = $5 WHERE id = $1 AND empresa_id = $2`,
		m.ID, r.empresaID, m.Nome, m.Telefone, m.Ativo)
	if err != nil {
		return fmt.Errorf("atualizar motorista: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}

func (r *Repositorio) DefinirAtivo(ctx context.Context, id int64, ativo bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE motorista SET ativo = $3 WHERE id = $1 AND empresa_id = $2`, id, r.empresaID, ativo)
	if err != nil {
		return fmt.Errorf("alterar motorista: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}
