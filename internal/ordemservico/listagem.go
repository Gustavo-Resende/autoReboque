package ordemservico

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Fase separa as duas telas: a de orçamentos e a de ordens de serviço.
type Fase string

const (
	FaseOrcamento Fase = "ORCAMENTO"
	FaseOS        Fase = "OS"
)

// Filtro são os critérios da tela de listagem. Campo vazio = não filtra.
type Filtro struct {
	Fase            Fase
	Status          Status // aceita o virtual OrcamentoExpirado
	StatusPagamento StatusPagamento
	ClienteID       int64

	// SomenteAguardando restringe a orçamentos abertos/enviados dentro da validade.
	SomenteAguardando bool

	// Busca procura em número (ex.: "142" ou "0142/2026"), nome do cliente e placa.
	Busca string

	Pagina         int // a partir de 1
	ItensPorPagina int
}

// Resumo são os totais do conjunto filtrado, para o topo da lista.
type Resumo struct {
	Quantidade    int
	TotalCentavos int64
	PagoCentavos  int64
}

// SaldoCentavos é o que ainda falta receber no conjunto.
func (r Resumo) SaldoCentavos() int64 {
	return r.TotalCentavos - r.PagoCentavos
}

// LinhaListagem é uma OS enxuta para a tabela: sem itens, com o total gravado.
type LinhaListagem struct {
	ID                int64
	Numero            Numero
	Versao            int
	Status            Status // já convertido para expirado quando for o caso
	StatusPagamento   StatusPagamento
	DataEmissao       time.Time
	ValidoAte         time.Time
	AgendadaPara      time.Time
	ClienteNome       string
	VeiculoModelo     string
	VeiculoPlaca      string
	MotoristaNome     string
	TotalCentavos     int64
	ValorPagoCentavos int64
}

func (l LinhaListagem) EhOrcamento() bool { return l.Status.EhOrcamento() }

// Listagem é o resultado paginado de Listar.
type Listagem struct {
	Linhas       []LinhaListagem
	Resumo       Resumo
	Pagina       int
	TotalPaginas int
}

func (l Listagem) TemAnterior() bool { return l.Pagina > 1 }
func (l Listagem) TemProxima() bool  { return l.Pagina < l.TotalPaginas }

// exprStatusExibido é o status como a pessoa vê, em SQL: orçamento aberto ou
// enviado com validade vencida conta como expirado.
const exprStatusExibido = `CASE
	WHEN status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO') AND valido_ate < CURRENT_DATE THEN 'ORCAMENTO_EXPIRADO'
	ELSE status END`

// Listar devolve as OS que batem com o filtro, mais recentes primeiro, e o
// resumo financeiro de todo o conjunto filtrado (não só da página).
func (r *Repositorio) Listar(ctx context.Context, f Filtro) (*Listagem, error) {
	if f.Pagina < 1 {
		f.Pagina = 1
	}
	if f.ItensPorPagina < 1 {
		f.ItensPorPagina = 50
	}

	// A cláusula WHERE é montada uma vez e usada nas duas consultas.
	// Os parâmetros são sempre passados ($n), nunca concatenados.
	condicoes := []string{"empresa_id = $1"}
	args := []any{r.empresaID}
	adicionar := func(cond string, valor any) {
		args = append(args, valor)
		condicoes = append(condicoes, fmt.Sprintf(cond, len(args)))
	}
	switch f.Fase {
	case FaseOrcamento:
		condicoes = append(condicoes, "status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO', 'ORCAMENTO_RECUSADO')")
	case FaseOS:
		condicoes = append(condicoes, "status IN ('AGENDADA', 'EM_ANDAMENTO', 'CONCLUIDA', 'CANCELADA')")
	}
	if f.Status != "" {
		adicionar(exprStatusExibido+" = $%d", string(f.Status))
	}
	if f.SomenteAguardando {
		condicoes = append(condicoes, "status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO') AND (valido_ate IS NULL OR valido_ate >= CURRENT_DATE)")
	}
	if f.StatusPagamento != "" {
		adicionar("status_pagamento = $%d", f.StatusPagamento)
	}
	if f.ClienteID != 0 {
		adicionar("cliente_id = $%d", f.ClienteID)
	}
	if busca := strings.TrimSpace(f.Busca); busca != "" {
		adicionar(`(
			unaccent(cliente_nome) ILIKE '%%' || unaccent($%[1]d) || '%%'
			OR veiculo_placa ILIKE '%%' || $%[1]d || '%%'
			OR unaccent(veiculo_modelo) ILIKE '%%' || unaccent($%[1]d) || '%%'
			OR lpad(numero::text, 4, '0') || '/' || ano::text LIKE '%%' || $%[1]d || '%%'
		)`, busca)
	}
	where := "WHERE " + strings.Join(condicoes, " AND ")

	var resumo Resumo
	err := r.pool.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(total_centavos), 0), coalesce(sum(valor_pago_centavos), 0)
		FROM ordem_servico `+where, args...,
	).Scan(&resumo.Quantidade, &resumo.TotalCentavos, &resumo.PagoCentavos)
	if err != nil {
		return nil, fmt.Errorf("resumo da listagem: %w", err)
	}

	argsPagina := append(append([]any{}, args...), f.ItensPorPagina, (f.Pagina-1)*f.ItensPorPagina)
	linhas, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, numero, ano, versao, %s, status_pagamento, data_emissao, valido_ate, agendada_para,
		       cliente_nome, veiculo_modelo, veiculo_placa, motorista_nome, total_centavos, valor_pago_centavos
		FROM ordem_servico %s
		ORDER BY ano DESC, numero DESC
		LIMIT $%d OFFSET $%d`, exprStatusExibido, where, len(args)+1, len(args)+2), argsPagina...)
	if err != nil {
		return nil, fmt.Errorf("listar OS: %w", err)
	}
	defer linhas.Close()

	lista := &Listagem{Resumo: resumo, Pagina: f.Pagina}
	for linhas.Next() {
		var l LinhaListagem
		var validoAte, agendadaPara *time.Time
		if err := linhas.Scan(
			&l.ID, &l.Numero.Sequencial, &l.Numero.Ano, &l.Versao, &l.Status, &l.StatusPagamento, &l.DataEmissao, &validoAte, &agendadaPara,
			&l.ClienteNome, &l.VeiculoModelo, &l.VeiculoPlaca, &l.MotoristaNome, &l.TotalCentavos, &l.ValorPagoCentavos,
		); err != nil {
			return nil, err
		}
		l.ValidoAte = tempoOuZero(validoAte)
		l.AgendadaPara = tempoOuZero(agendadaPara)
		lista.Linhas = append(lista.Linhas, l)
	}
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	lista.TotalPaginas = (resumo.Quantidade + f.ItensPorPagina - 1) / f.ItensPorPagina
	if lista.TotalPaginas == 0 {
		lista.TotalPaginas = 1
	}
	return lista, nil
}

// ContarPorStatus devolve quantas OS há em cada status exibido (para as abas).
func (r *Repositorio) ContarPorStatus(ctx context.Context) (map[Status]int, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT `+exprStatusExibido+`, count(*) FROM ordem_servico WHERE empresa_id = $1 GROUP BY 1`, r.empresaID)
	if err != nil {
		return nil, fmt.Errorf("contar por status: %w", err)
	}
	defer linhas.Close()
	contagem := map[Status]int{}
	for linhas.Next() {
		var s Status
		var n int
		if err := linhas.Scan(&s, &n); err != nil {
			return nil, err
		}
		contagem[s] = n
	}
	return contagem, linhas.Err()
}
