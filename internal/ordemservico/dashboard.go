package ordemservico

import (
	"context"
	"fmt"
	"time"
)

// Periodo é um intervalo de datas inclusivo, usado pelo dashboard.
type Periodo struct {
	De  time.Time
	Ate time.Time
}

// MesDe devolve o período do mês em que a data cai.
func MesDe(t time.Time) Periodo {
	inicio := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return Periodo{De: inicio, Ate: inicio.AddDate(0, 1, -1)}
}

// ValorMensal é uma barra do gráfico de faturamento.
type ValorMensal struct {
	Mes        time.Time
	Centavos   int64
	Quantidade int
}

// ValorPorForma é uma linha do quadro de formas de pagamento.
type ValorPorForma struct {
	Forma      FormaPagamento
	Centavos   int64
	Quantidade int
}

// Indicadores são os números do dashboard.
//
// Só a fase de OS conta dinheiro (agendada, em andamento, concluída). Orçamento
// é rascunho: aparece à parte, como "aguardando resposta". Cancelada não conta.
type Indicadores struct {
	Periodo Periodo

	// No período, por data de emissão.
	OSQuantidade        int
	FaturadoCentavos    int64
	RecebidoCentavos    int64
	TicketMedioCentavos int64
	Aprovadas           int // orçamentos que viraram OS
	Recusadas           int
	ConversaoPercentual int // aprovadas / (aprovadas + recusadas)

	// Situação atual, independente do período.
	AReceberCentavos          int64
	AReceberQuantidade        int
	OrcamentosAbertos         int
	OrcamentosAbertosCentavos int64
	OrcamentosExpirados       int
	Agendadas                 int
	EmAndamento               int
	ConcluidasHoje            int

	FaturamentoMensal []ValorMensal // últimos 6 meses, do mais antigo ao mais novo
	PorFormaPagamento []ValorPorForma
}

// faseOSAtiva são os status que contam como faturamento.
const faseOSAtiva = `status IN ('AGENDADA', 'EM_ANDAMENTO', 'CONCLUIDA')`

// Dashboard calcula os indicadores para o período.
func (r *Repositorio) Dashboard(ctx context.Context, p Periodo) (*Indicadores, error) {
	ind := &Indicadores{Periodo: p}

	err := r.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE `+faseOSAtiva+`),
			coalesce(sum(total_centavos) FILTER (WHERE `+faseOSAtiva+`), 0),
			coalesce(sum(valor_pago_centavos) FILTER (WHERE `+faseOSAtiva+`), 0),
			count(*) FILTER (WHERE status = 'ORCAMENTO_RECUSADO')
		FROM ordem_servico
		WHERE empresa_id = $1 AND data_emissao BETWEEN $2 AND $3`,
		r.empresaID, p.De, p.Ate,
	).Scan(&ind.OSQuantidade, &ind.FaturadoCentavos, &ind.RecebidoCentavos, &ind.Recusadas)
	if err != nil {
		return nil, fmt.Errorf("indicadores do período: %w", err)
	}
	ind.Aprovadas = ind.OSQuantidade
	if ind.OSQuantidade > 0 {
		ind.TicketMedioCentavos = ind.FaturadoCentavos / int64(ind.OSQuantidade)
	}
	if ind.Aprovadas+ind.Recusadas > 0 {
		ind.ConversaoPercentual = ind.Aprovadas * 100 / (ind.Aprovadas + ind.Recusadas)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE `+faseOSAtiva+` AND status_pagamento <> 'PAGA'),
			coalesce(sum(greatest(total_centavos - valor_pago_centavos, 0)) FILTER (WHERE `+faseOSAtiva+` AND status_pagamento <> 'PAGA'), 0),
			count(*) FILTER (WHERE status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO') AND (valido_ate IS NULL OR valido_ate >= CURRENT_DATE)),
			coalesce(sum(total_centavos) FILTER (WHERE status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO') AND (valido_ate IS NULL OR valido_ate >= CURRENT_DATE)), 0),
			count(*) FILTER (WHERE status IN ('ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO') AND valido_ate < CURRENT_DATE),
			count(*) FILTER (WHERE status = 'AGENDADA'),
			count(*) FILTER (WHERE status = 'EM_ANDAMENTO'),
			count(*) FILTER (WHERE status = 'CONCLUIDA' AND concluida_em::date = CURRENT_DATE)
		FROM ordem_servico
		WHERE empresa_id = $1`,
		r.empresaID,
	).Scan(&ind.AReceberQuantidade, &ind.AReceberCentavos, &ind.OrcamentosAbertos, &ind.OrcamentosAbertosCentavos,
		&ind.OrcamentosExpirados, &ind.Agendadas, &ind.EmAndamento, &ind.ConcluidasHoje)
	if err != nil {
		return nil, fmt.Errorf("indicadores atuais: %w", err)
	}

	// Faturamento dos últimos 6 meses, terminando no mês do fim do período.
	// Meses sem OS entram com zero para o gráfico não ficar com buracos.
	ultimoMes := time.Date(p.Ate.Year(), p.Ate.Month(), 1, 0, 0, 0, 0, time.UTC)
	primeiroMes := ultimoMes.AddDate(0, -5, 0)
	porMes := map[time.Time]ValorMensal{}
	linhas, err := r.pool.Query(ctx, `
		SELECT date_trunc('month', data_emissao)::date, sum(total_centavos), count(*)
		FROM ordem_servico
		WHERE empresa_id = $1 AND `+faseOSAtiva+` AND data_emissao >= $2 AND data_emissao <= $3
		GROUP BY 1`,
		r.empresaID, primeiroMes, ultimoMes.AddDate(0, 1, -1))
	if err != nil {
		return nil, fmt.Errorf("faturamento mensal: %w", err)
	}
	for linhas.Next() {
		var v ValorMensal
		if err := linhas.Scan(&v.Mes, &v.Centavos, &v.Quantidade); err != nil {
			linhas.Close()
			return nil, err
		}
		porMes[time.Date(v.Mes.Year(), v.Mes.Month(), 1, 0, 0, 0, 0, time.UTC)] = v
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	for mes := primeiroMes; !mes.After(ultimoMes); mes = mes.AddDate(0, 1, 0) {
		v := porMes[mes]
		v.Mes = mes
		ind.FaturamentoMensal = append(ind.FaturamentoMensal, v)
	}

	linhas, err = r.pool.Query(ctx, `
		SELECT forma_pagamento, sum(valor_pago_centavos), count(*)
		FROM ordem_servico
		WHERE empresa_id = $1 AND `+faseOSAtiva+` AND valor_pago_centavos > 0 AND data_emissao BETWEEN $2 AND $3
		GROUP BY 1 ORDER BY 2 DESC`,
		r.empresaID, p.De, p.Ate)
	if err != nil {
		return nil, fmt.Errorf("por forma de pagamento: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var v ValorPorForma
		if err := linhas.Scan(&v.Forma, &v.Centavos, &v.Quantidade); err != nil {
			return nil, err
		}
		ind.PorFormaPagamento = append(ind.PorFormaPagamento, v)
	}
	return ind, linhas.Err()
}

// MaiorFaturamentoMensal é o teto do gráfico de barras (evita divisão por zero no template).
func (i *Indicadores) MaiorFaturamentoMensal() int64 {
	var maior int64 = 1
	for _, v := range i.FaturamentoMensal {
		if v.Centavos > maior {
			maior = v.Centavos
		}
	}
	return maior
}

// TotalRecebidoPorForma é a base para as porcentagens do quadro de formas.
func (i *Indicadores) TotalRecebidoPorForma() int64 {
	var total int64 = 1
	for _, v := range i.PorFormaPagamento {
		total += v.Centavos
	}
	return total
}
