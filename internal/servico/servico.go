// Package servico é o catálogo do que a empresa cobra: "Saída de base",
// "Quilometragem rodada", "Hora parada"... com unidade e preço padrão.
//
// A OS copia descrição, unidade e preço para o item na hora de adicionar;
// mudar o catálogo depois não altera OS antigas.
package servico

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
	ErrNaoEncontrado   = errors.New("serviço não encontrado")
	ErrNomeObrigatorio = errors.New("informe o nome do serviço")
)

type Unidade string

const (
	UN     Unidade = "UN"
	KM     Unidade = "KM"
	HORA   Unidade = "HORA"
	DIARIA Unidade = "DIARIA"
)

var TodasUnidades = []Unidade{UN, KM, HORA, DIARIA}

func (u Unidade) Valida() bool {
	for _, o := range TodasUnidades {
		if o == u {
			return true
		}
	}
	return false
}

func (u Unidade) Rotulo() string {
	switch u {
	case KM:
		return "Quilômetro"
	case HORA:
		return "Hora"
	case DIARIA:
		return "Diária"
	}
	return "Unidade"
}

type Servico struct {
	ID             int64
	Nome           string
	Unidade        Unidade
	PrecoCentavos  int64
	QuantidadeDoKm bool // a quantidade vem do km do trajeto da OS
	Ativo          bool
	Ordem          int
	CriadoEm       time.Time
	AtualizadoEm   time.Time
}

func Novo() *Servico {
	return &Servico{Unidade: UN, Ativo: true}
}

func (s *Servico) Validar() error {
	s.Nome = strings.TrimSpace(s.Nome)
	if s.Nome == "" {
		return ErrNomeObrigatorio
	}
	if !s.Unidade.Valida() {
		return fmt.Errorf("unidade inválida: %q", s.Unidade)
	}
	if s.PrecoCentavos < 0 {
		return errors.New("o preço não pode ser negativo")
	}
	if s.Unidade != KM {
		s.QuantidadeDoKm = false
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

const colunas = `id, nome, unidade, preco_centavos, quantidade_do_km, ativo, ordem, criado_em, atualizado_em`

func ler(linha pgx.Row) (*Servico, error) {
	var s Servico
	err := linha.Scan(&s.ID, &s.Nome, &s.Unidade, &s.PrecoCentavos, &s.QuantidadeDoKm, &s.Ativo, &s.Ordem, &s.CriadoEm, &s.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrado
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repositorio) Buscar(ctx context.Context, id int64) (*Servico, error) {
	return ler(r.pool.QueryRow(ctx, `SELECT `+colunas+` FROM servico WHERE id = $1 AND empresa_id = $2`, id, r.empresaID))
}

// Listar devolve os serviços na ordem de exibição. Com somenteAtivos, esconde os desativados.
func (r *Repositorio) Listar(ctx context.Context, somenteAtivos bool) ([]Servico, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT `+colunas+` FROM servico WHERE empresa_id = $1 AND ($2 = false OR ativo = true) ORDER BY ordem, lower(nome)`,
		r.empresaID, somenteAtivos)
	if err != nil {
		return nil, fmt.Errorf("listar serviços: %w", err)
	}
	defer linhas.Close()
	var lista []Servico
	for linhas.Next() {
		s, err := ler(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, *s)
	}
	return lista, linhas.Err()
}

func (r *Repositorio) Criar(ctx context.Context, s *Servico) error {
	if err := s.Validar(); err != nil {
		return err
	}
	// Sem ordem informada, entra no fim da lista.
	if s.Ordem == 0 {
		if err := r.pool.QueryRow(ctx,
			`SELECT coalesce(max(ordem), 0) + 1 FROM servico WHERE empresa_id = $1`, r.empresaID).Scan(&s.Ordem); err != nil {
			return fmt.Errorf("próxima ordem: %w", err)
		}
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO servico (empresa_id, nome, unidade, preco_centavos, quantidade_do_km, ativo, ordem)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, criado_em, atualizado_em`,
		r.empresaID, s.Nome, s.Unidade, s.PrecoCentavos, s.QuantidadeDoKm, s.Ativo, s.Ordem,
	).Scan(&s.ID, &s.CriadoEm, &s.AtualizadoEm)
	if err != nil {
		return fmt.Errorf("criar serviço: %w", err)
	}
	return nil
}

func (r *Repositorio) Atualizar(ctx context.Context, s *Servico) error {
	if err := s.Validar(); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE servico SET nome = $3, unidade = $4, preco_centavos = $5, quantidade_do_km = $6, ativo = $7, ordem = $8, atualizado_em = now()
		WHERE id = $1 AND empresa_id = $2`,
		s.ID, r.empresaID, s.Nome, s.Unidade, s.PrecoCentavos, s.QuantidadeDoKm, s.Ativo, s.Ordem)
	if err != nil {
		return fmt.Errorf("atualizar serviço: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}

// DefinirAtivo ativa ou desativa (nunca apaga: itens de OS antigas apontam para ele).
func (r *Repositorio) DefinirAtivo(ctx context.Context, id int64, ativo bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE servico SET ativo = $3, atualizado_em = now() WHERE id = $1 AND empresa_id = $2`, id, r.empresaID, ativo)
	if err != nil {
		return fmt.Errorf("alterar serviço: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}
