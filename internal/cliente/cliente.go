// Package cliente é o cadastro de quem contrata o serviço: pessoa física na
// estrada, oficina, locadora, órgão público.
//
// O cliente 1 é o "Serviço particular": um registro de sistema usado quando
// não há dados de quem contratou. Não se edita nem se desativa.
package cliente

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNaoEncontrado      = errors.New("cliente não encontrado")
	ErrDocumentoDuplicado = errors.New("já existe um cliente com este CPF/CNPJ")
	ErrClienteDeSistema   = errors.New("o cliente padrão do sistema não pode ser alterado")
	ErrNomeObrigatorio    = errors.New("informe o nome do cliente")
)

// ParticularID é o id do cliente de sistema "Serviço particular".
const ParticularID int64 = 1

type Tipo string

const (
	PessoaFisica   Tipo = "PF"
	PessoaJuridica Tipo = "PJ"
)

func (t Tipo) Rotulo() string {
	if t == PessoaJuridica {
		return "Pessoa jurídica"
	}
	return "Pessoa física"
}

// RotuloDocumento diz qual documento se espera para o tipo.
func (t Tipo) RotuloDocumento() string {
	if t == PessoaJuridica {
		return "CNPJ"
	}
	return "CPF"
}

type Categoria string

var TodasCategorias = []Categoria{"PARTICULAR", "OFICINA", "LOCADORA", "CONCESSIONARIA", "SEGURADORA", "ORGAO_PUBLICO", "OUTRO"}

func (c Categoria) Rotulo() string {
	switch c {
	case "PARTICULAR":
		return "Particular"
	case "OFICINA":
		return "Oficina"
	case "LOCADORA":
		return "Locadora"
	case "CONCESSIONARIA":
		return "Concessionária"
	case "SEGURADORA":
		return "Seguradora"
	case "ORGAO_PUBLICO":
		return "Órgão público"
	case "OUTRO":
		return "Outro"
	}
	return string(c)
}

func (c Categoria) Valida() bool {
	for _, o := range TodasCategorias {
		if o == c {
			return true
		}
	}
	return false
}

type Cliente struct {
	ID           int64
	Tipo         Tipo
	Nome         string
	Documento    string
	Telefone     string
	Email        string
	Endereco     string
	Cidade       string
	Categoria    Categoria
	Observacoes  string
	Ativo        bool
	Sistema      bool
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Novo devolve um cliente com os padrões: pessoa física, particular, ativo.
func Novo() *Cliente {
	return &Cliente{Tipo: PessoaFisica, Categoria: "PARTICULAR", Ativo: true}
}

// Validar confere o que não depende do banco e normaliza espaços.
func (c *Cliente) Validar() error {
	c.Nome = strings.TrimSpace(c.Nome)
	c.Documento = strings.TrimSpace(c.Documento)
	c.Telefone = strings.TrimSpace(c.Telefone)
	c.Email = strings.TrimSpace(c.Email)
	c.Endereco = strings.TrimSpace(c.Endereco)
	c.Cidade = strings.TrimSpace(c.Cidade)
	c.Observacoes = strings.TrimSpace(c.Observacoes)
	if c.Nome == "" {
		return ErrNomeObrigatorio
	}
	if c.Tipo != PessoaFisica && c.Tipo != PessoaJuridica {
		return fmt.Errorf("tipo inválido: %q", c.Tipo)
	}
	if !c.Categoria.Valida() {
		return fmt.Errorf("categoria inválida: %q", c.Categoria)
	}
	return nil
}

// TelefoneDigitos devolve só os números, para montar links de WhatsApp.
func (c Cliente) TelefoneDigitos() string {
	return SomenteDigitos(c.Telefone)
}

// SomenteDigitos remove tudo que não é número.
func SomenteDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type Repositorio struct {
	pool      *pgxpool.Pool
	empresaID int64
}

func NovoRepositorio(pool *pgxpool.Pool, empresaID int64) *Repositorio {
	return &Repositorio{pool: pool, empresaID: empresaID}
}

const colunas = `id, tipo, nome, documento, telefone, email, endereco, cidade, categoria, observacoes, ativo, sistema, criado_em, atualizado_em`

func ler(linha pgx.Row) (*Cliente, error) {
	var c Cliente
	err := linha.Scan(&c.ID, &c.Tipo, &c.Nome, &c.Documento, &c.Telefone, &c.Email, &c.Endereco, &c.Cidade,
		&c.Categoria, &c.Observacoes, &c.Ativo, &c.Sistema, &c.CriadoEm, &c.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrado
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repositorio) Buscar(ctx context.Context, id int64) (*Cliente, error) {
	return ler(r.pool.QueryRow(ctx, `SELECT `+colunas+` FROM cliente WHERE id = $1 AND empresa_id = $2`, id, r.empresaID))
}

func (r *Repositorio) Criar(ctx context.Context, c *Cliente) error {
	if err := c.Validar(); err != nil {
		return err
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cliente (empresa_id, tipo, nome, documento, telefone, email, endereco, cidade, categoria, observacoes, ativo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, criado_em, atualizado_em`,
		r.empresaID, c.Tipo, c.Nome, c.Documento, c.Telefone, c.Email, c.Endereco, c.Cidade, c.Categoria, c.Observacoes, c.Ativo,
	).Scan(&c.ID, &c.CriadoEm, &c.AtualizadoEm)
	if err != nil {
		return traduzirErro(err, "criar cliente")
	}
	return nil
}

func (r *Repositorio) Atualizar(ctx context.Context, c *Cliente) error {
	if err := c.Validar(); err != nil {
		return err
	}
	if c.Sistema {
		return ErrClienteDeSistema
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE cliente SET
			tipo = $3, nome = $4, documento = $5, telefone = $6, email = $7, endereco = $8, cidade = $9,
			categoria = $10, observacoes = $11, ativo = $12, atualizado_em = now()
		WHERE id = $1 AND empresa_id = $2 AND sistema = false`,
		c.ID, r.empresaID, c.Tipo, c.Nome, c.Documento, c.Telefone, c.Email, c.Endereco, c.Cidade,
		c.Categoria, c.Observacoes, c.Ativo)
	if err != nil {
		return traduzirErro(err, "atualizar cliente")
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}

// DefinirAtivo ativa ou desativa um cliente (nunca se apaga: as OS apontam para ele).
func (r *Repositorio) DefinirAtivo(ctx context.Context, id int64, ativo bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE cliente SET ativo = $3, atualizado_em = now() WHERE id = $1 AND empresa_id = $2 AND sistema = false`,
		id, r.empresaID, ativo)
	if err != nil {
		return fmt.Errorf("alterar cliente: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrado
	}
	return nil
}

// Filtro da listagem de clientes.
type Filtro struct {
	Busca           string
	Categoria       Categoria
	IncluirInativos bool
	Pagina          int
	ItensPorPagina  int
}

type Listagem struct {
	Clientes     []Cliente
	Total        int
	Pagina       int
	TotalPaginas int
}

func (l Listagem) TemAnterior() bool { return l.Pagina > 1 }
func (l Listagem) TemProxima() bool  { return l.Pagina < l.TotalPaginas }

// Listar devolve os clientes em ordem alfabética, com o de sistema por último.
func (r *Repositorio) Listar(ctx context.Context, f Filtro) (*Listagem, error) {
	if f.Pagina < 1 {
		f.Pagina = 1
	}
	if f.ItensPorPagina < 1 {
		f.ItensPorPagina = 50
	}
	condicoes := []string{"empresa_id = $1"}
	args := []any{r.empresaID}
	if !f.IncluirInativos {
		condicoes = append(condicoes, "ativo = true")
	}
	if f.Categoria != "" {
		args = append(args, f.Categoria)
		condicoes = append(condicoes, fmt.Sprintf("categoria = $%d", len(args)))
	}
	if busca := strings.TrimSpace(f.Busca); busca != "" {
		args = append(args, busca)
		condicoes = append(condicoes, fmt.Sprintf(`(
			unaccent(nome) ILIKE '%%' || unaccent($%[1]d) || '%%'
			OR documento ILIKE '%%' || $%[1]d || '%%'
			OR telefone ILIKE '%%' || $%[1]d || '%%'
			OR unaccent(cidade) ILIKE '%%' || unaccent($%[1]d) || '%%'
		)`, len(args)))
	}
	where := "WHERE " + strings.Join(condicoes, " AND ")

	lista := &Listagem{Pagina: f.Pagina}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM cliente `+where, args...).Scan(&lista.Total); err != nil {
		return nil, fmt.Errorf("contar clientes: %w", err)
	}
	argsPagina := append(append([]any{}, args...), f.ItensPorPagina, (f.Pagina-1)*f.ItensPorPagina)
	linhas, err := r.pool.Query(ctx, fmt.Sprintf(
		`SELECT `+colunas+` FROM cliente %s ORDER BY sistema, lower(nome) LIMIT $%d OFFSET $%d`,
		where, len(args)+1, len(args)+2), argsPagina...)
	if err != nil {
		return nil, fmt.Errorf("listar clientes: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		c, err := ler(linhas)
		if err != nil {
			return nil, err
		}
		lista.Clientes = append(lista.Clientes, *c)
	}
	lista.TotalPaginas = max(1, (lista.Total+f.ItensPorPagina-1)/f.ItensPorPagina)
	return lista, linhas.Err()
}

// Sugerir é a busca rápida do formulário de OS: até `limite` clientes ativos
// cujo nome, documento ou telefone contenham o texto.
func (r *Repositorio) Sugerir(ctx context.Context, texto string, limite int) ([]Cliente, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, nil
	}
	linhas, err := r.pool.Query(ctx, `
		SELECT `+colunas+` FROM cliente
		WHERE empresa_id = $1 AND ativo = true AND sistema = false
		  AND (unaccent(nome) ILIKE '%' || unaccent($2) || '%' OR documento ILIKE '%' || $2 || '%' OR telefone ILIKE '%' || $2 || '%')
		ORDER BY lower(nome) LIMIT $3`, r.empresaID, texto, limite)
	if err != nil {
		return nil, fmt.Errorf("sugerir clientes: %w", err)
	}
	defer linhas.Close()
	var lista []Cliente
	for linhas.Next() {
		c, err := ler(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, *c)
	}
	return lista, linhas.Err()
}

// traduzirErro converte a violação do índice único de documento em ErrDocumentoDuplicado.
func traduzirErro(err error, contexto string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "cliente_documento_unico" {
		return ErrDocumentoDuplicado
	}
	return fmt.Errorf("%s: %w", contexto, err)
}
