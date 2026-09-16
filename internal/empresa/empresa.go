// Package empresa guarda os dados da guincheira dona do sistema: o que vai no
// cabeçalho dos documentos, o logo, a cor da marca e as regras de orçamento.
package empresa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gustavo-Resende/autoSocorro/internal/storage"
)

// CorPadrao é o laranja da Auto Socorro Trevo.
const CorPadrao = "#F4511E"

var regexCorHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Empresa struct {
	ID                    int64
	NomeFantasia          string
	RazaoSocial           string
	CNPJ                  string
	Endereco              string
	Cidade                string
	Telefone              string
	WhatsApp              string
	Email                 string
	LogoChave             string // chave no Storage; "" = sem logo
	CorPrimaria           string // "#RRGGBB"
	OrcamentoValidadeDias int
	OrcamentoCondicoes    string
	AtualizadaEm          time.Time
}

// TemLogo é um atalho para os templates.
func (e Empresa) TemLogo() bool { return e.LogoChave != "" }

// NomeExibicao é o nome fantasia ou, na falta dele, a razão social.
func (e Empresa) NomeExibicao() string {
	if e.NomeFantasia != "" {
		return e.NomeFantasia
	}
	return e.RazaoSocial
}

// Validar normaliza e confere os campos editáveis.
func (e *Empresa) Validar() error {
	e.NomeFantasia = strings.TrimSpace(e.NomeFantasia)
	e.RazaoSocial = strings.TrimSpace(e.RazaoSocial)
	e.CNPJ = strings.TrimSpace(e.CNPJ)
	e.Endereco = strings.TrimSpace(e.Endereco)
	e.Cidade = strings.TrimSpace(e.Cidade)
	e.Telefone = strings.TrimSpace(e.Telefone)
	e.WhatsApp = strings.TrimSpace(e.WhatsApp)
	e.Email = strings.TrimSpace(e.Email)
	e.CorPrimaria = strings.ToUpper(strings.TrimSpace(e.CorPrimaria))
	e.OrcamentoCondicoes = strings.TrimSpace(e.OrcamentoCondicoes)
	if e.NomeFantasia == "" && e.RazaoSocial == "" {
		return errors.New("informe o nome fantasia ou a razão social")
	}
	if e.CorPrimaria == "" {
		e.CorPrimaria = CorPadrao
	}
	if !regexCorHex.MatchString(e.CorPrimaria) {
		return errors.New("cor inválida: use o formato #RRGGBB")
	}
	if e.OrcamentoValidadeDias < 1 || e.OrcamentoValidadeDias > 365 {
		return errors.New("validade do orçamento deve ficar entre 1 e 365 dias")
	}
	return nil
}

type Repositorio struct {
	pool      *pgxpool.Pool
	empresaID int64
	storage   storage.Storage
}

func NovoRepositorio(pool *pgxpool.Pool, empresaID int64, st storage.Storage) *Repositorio {
	return &Repositorio{pool: pool, empresaID: empresaID, storage: st}
}

func (r *Repositorio) Buscar(ctx context.Context) (*Empresa, error) {
	var e Empresa
	err := r.pool.QueryRow(ctx, `
		SELECT id, nome_fantasia, razao_social, cnpj, endereco, cidade, telefone, whatsapp, email,
		       logo_chave, cor_primaria, orcamento_validade_dias, orcamento_condicoes, atualizada_em
		FROM empresa WHERE id = $1`, r.empresaID,
	).Scan(&e.ID, &e.NomeFantasia, &e.RazaoSocial, &e.CNPJ, &e.Endereco, &e.Cidade, &e.Telefone, &e.WhatsApp, &e.Email,
		&e.LogoChave, &e.CorPrimaria, &e.OrcamentoValidadeDias, &e.OrcamentoCondicoes, &e.AtualizadaEm)
	if err != nil {
		return nil, fmt.Errorf("ler dados da empresa: %w", err)
	}
	return &e, nil
}

// Atualizar grava os campos editáveis. O logo tem fluxo próprio.
func (r *Repositorio) Atualizar(ctx context.Context, e *Empresa) error {
	if err := e.Validar(); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE empresa SET
			nome_fantasia = $2, razao_social = $3, cnpj = $4, endereco = $5, cidade = $6, telefone = $7, whatsapp = $8, email = $9,
			cor_primaria = $10, orcamento_validade_dias = $11, orcamento_condicoes = $12, atualizada_em = now()
		WHERE id = $1`,
		r.empresaID, e.NomeFantasia, e.RazaoSocial, e.CNPJ, e.Endereco, e.Cidade, e.Telefone, e.WhatsApp, e.Email,
		e.CorPrimaria, e.OrcamentoValidadeDias, e.OrcamentoCondicoes)
	if err != nil {
		return fmt.Errorf("gravar dados da empresa: %w", err)
	}
	return nil
}

// AtualizarLogo guarda a imagem no Storage e registra a chave.
// A chave muda a cada envio (tem a hora no nome) para o navegador não
// mostrar o logo antigo por causa de cache.
func (r *Repositorio) AtualizarLogo(ctx context.Context, extensao string, conteudo io.Reader) error {
	chave := fmt.Sprintf("empresa/%d/logo-%d%s", r.empresaID, time.Now().Unix(), extensao)
	if err := r.storage.Salvar(ctx, chave, conteudo); err != nil {
		return fmt.Errorf("salvar logo: %w", err)
	}

	var anterior string
	err := r.pool.QueryRow(ctx,
		`UPDATE empresa SET logo_chave = $2, atualizada_em = now() WHERE id = $1
		 RETURNING (SELECT logo_chave FROM empresa WHERE id = $1)`, r.empresaID, chave).Scan(&anterior)
	if err != nil {
		return fmt.Errorf("registrar logo: %w", err)
	}
	if anterior != "" && anterior != chave {
		_ = r.storage.Remover(ctx, anterior) // melhor esforço: o registro já está certo
	}
	return nil
}

// RemoverLogo apaga o logo atual.
func (r *Repositorio) RemoverLogo(ctx context.Context) error {
	var chave string
	err := r.pool.QueryRow(ctx,
		`UPDATE empresa SET logo_chave = '', atualizada_em = now() WHERE id = $1
		 RETURNING (SELECT logo_chave FROM empresa WHERE id = $1)`, r.empresaID).Scan(&chave)
	if err != nil {
		return fmt.Errorf("remover logo: %w", err)
	}
	if chave != "" {
		_ = r.storage.Remover(ctx, chave)
	}
	return nil
}

// AbrirLogo devolve o conteúdo do logo atual.
func (r *Repositorio) AbrirLogo(ctx context.Context) (io.ReadCloser, error) {
	e, err := r.Buscar(ctx)
	if err != nil {
		return nil, err
	}
	if e.LogoChave == "" {
		return nil, storage.ErrNaoEncontrado
	}
	return r.storage.Abrir(ctx, e.LogoChave)
}
