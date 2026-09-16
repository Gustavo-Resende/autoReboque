package ordemservico

import "fmt"

// Status é o andamento do documento. Os valores são os gravados no banco.
//
// Os três primeiros são a fase de ORÇAMENTO; os demais, a fase de OS.
// É o mesmo registro e o mesmo número o tempo todo — só o status muda.
type Status string

const (
	OrcamentoAberto   Status = "ORCAMENTO_ABERTO"
	OrcamentoEnviado  Status = "ORCAMENTO_ENVIADO"
	OrcamentoRecusado Status = "ORCAMENTO_RECUSADO"
	Agendada          Status = "AGENDADA"
	EmAndamento       Status = "EM_ANDAMENTO"
	Concluida         Status = "CONCLUIDA"
	Cancelada         Status = "CANCELADA"

	// OrcamentoExpirado nunca é gravado: é derivado da validade (ver OS.StatusExibido).
	// Existe para exibição, filtro e contagem.
	OrcamentoExpirado Status = "ORCAMENTO_EXPIRADO"
)

// StatusDeOrcamento e StatusDeOS listam os status de cada fase, na ordem das abas.
var (
	StatusDeOrcamento = []Status{OrcamentoAberto, OrcamentoEnviado, OrcamentoExpirado, OrcamentoRecusado}
	StatusDeOS        = []Status{Agendada, EmAndamento, Concluida, Cancelada}
)

// transicoes diz, para cada status, para quais outros ele pode ir.
//
//	ORCAMENTO_ABERTO   → ENVIADO, AGENDADA (aprovar e agendar), EM_ANDAMENTO (aprovar e iniciar), RECUSADO
//	ORCAMENTO_ENVIADO  → AGENDADA, EM_ANDAMENTO, RECUSADO
//	ORCAMENTO_RECUSADO → ABERTO (reabrir)
//	ORCAMENTO_EXPIRADO → ABERTO (renovar validade), AGENDADA, EM_ANDAMENTO, RECUSADO
//	AGENDADA           → EM_ANDAMENTO, CANCELADA
//	EM_ANDAMENTO       → CONCLUIDA, CANCELADA
//	CONCLUIDA          → EM_ANDAMENTO ("reabrir": corrige um toque errado)
//	CANCELADA          → nada
var transicoes = map[Status][]Status{
	OrcamentoAberto:   {OrcamentoEnviado, Agendada, EmAndamento, OrcamentoRecusado},
	OrcamentoEnviado:  {Agendada, EmAndamento, OrcamentoRecusado},
	OrcamentoRecusado: {OrcamentoAberto},
	OrcamentoExpirado: {OrcamentoAberto, Agendada, EmAndamento, OrcamentoRecusado},
	Agendada:          {EmAndamento, Cancelada},
	EmAndamento:       {Concluida, Cancelada},
	Concluida:         {EmAndamento},
	Cancelada:         {},
}

// Valido diz se o status pode ser gravado (o expirado é virtual).
func (s Status) Valido() bool {
	_, ok := transicoes[s]
	return ok && s != OrcamentoExpirado
}

// EhOrcamento diz se o status pertence à fase de orçamento.
func (s Status) EhOrcamento() bool {
	switch s {
	case OrcamentoAberto, OrcamentoEnviado, OrcamentoRecusado, OrcamentoExpirado:
		return true
	}
	return false
}

// Ativo diz se o documento ainda "conta": não foi recusado nem cancelado.
func (s Status) Ativo() bool {
	return s != OrcamentoRecusado && s != Cancelada
}

// Rotulo é o texto do status para exibição.
func (s Status) Rotulo() string {
	switch s {
	case OrcamentoAberto:
		return "Aberto"
	case OrcamentoEnviado:
		return "Enviado"
	case OrcamentoRecusado:
		return "Recusado"
	case OrcamentoExpirado:
		return "Expirado"
	case Agendada:
		return "Agendada"
	case EmAndamento:
		return "Em andamento"
	case Concluida:
		return "Concluída"
	case Cancelada:
		return "Cancelada"
	}
	return string(s)
}

// Transicoes devolve os status para os quais este pode ir.
func (s Status) Transicoes() []Status {
	return transicoes[s]
}

// PodeIrPara diz se a transição s → destino é permitida.
func (s Status) PodeIrPara(destino Status) bool {
	for _, t := range transicoes[s] {
		if t == destino {
			return true
		}
	}
	return false
}

// RotuloAcao é o texto do botão que leva a este status, visto de onde se está.
func (s Status) RotuloAcao(de Status) string {
	switch s {
	case OrcamentoEnviado:
		return "Marcar como enviado"
	case OrcamentoRecusado:
		return "Recusado pelo cliente"
	case OrcamentoAberto:
		if de == OrcamentoExpirado {
			return "Renovar validade"
		}
		return "Reabrir orçamento"
	case Agendada:
		if de.EhOrcamento() {
			return "Aprovar e agendar"
		}
		return "Agendar"
	case EmAndamento:
		switch {
		case de.EhOrcamento():
			return "Aprovar e iniciar"
		case de == Concluida:
			return "Reabrir"
		}
		return "Iniciar serviço"
	case Concluida:
		return "Concluir"
	case Cancelada:
		return "Cancelar OS"
	}
	return string(s)
}

// Destrutiva marca ações que merecem confirmação.
func (s Status) Destrutiva() bool {
	return s == Cancelada || s == OrcamentoRecusado
}

// StatusPagamento é o eixo financeiro, independente do andamento do serviço.
type StatusPagamento string

const (
	Pendente StatusPagamento = "PENDENTE"
	Parcial  StatusPagamento = "PARCIAL"
	Paga     StatusPagamento = "PAGA"
)

var TodosStatusPagamento = []StatusPagamento{Pendente, Parcial, Paga}

func (s StatusPagamento) Valido() bool {
	switch s {
	case Pendente, Parcial, Paga:
		return true
	}
	return false
}

func (s StatusPagamento) Rotulo() string {
	switch s {
	case Pendente:
		return "Pendente"
	case Parcial:
		return "Parcial"
	case Paga:
		return "Paga"
	}
	return string(s)
}

// FormaPagamento é a lista fixa de formas aceitas.
type FormaPagamento string

const (
	FormaNenhuma  FormaPagamento = ""
	PIX           FormaPagamento = "PIX"
	Dinheiro      FormaPagamento = "DINHEIRO"
	Debito        FormaPagamento = "DEBITO"
	Credito       FormaPagamento = "CREDITO"
	Transferencia FormaPagamento = "TRANSFERENCIA"
	Faturado      FormaPagamento = "FATURADO" // a prazo, com vencimento
)

var TodasFormasPagamento = []FormaPagamento{PIX, Dinheiro, Debito, Credito, Transferencia, Faturado}

func (f FormaPagamento) Valida() bool {
	if f == FormaNenhuma {
		return true
	}
	for _, o := range TodasFormasPagamento {
		if o == f {
			return true
		}
	}
	return false
}

func (f FormaPagamento) Rotulo() string {
	switch f {
	case PIX:
		return "PIX"
	case Dinheiro:
		return "Dinheiro"
	case Debito:
		return "Cartão de débito"
	case Credito:
		return "Cartão de crédito"
	case Transferencia:
		return "Transferência"
	case Faturado:
		return "Faturado / a prazo"
	}
	return string(f)
}

// CategoriaVeiculo é o porte do veículo, que define o guincho e o preço:
// leve (moto, carro de passeio), utilitário (caminhonete, van) e
// extrapesado (caminhão, máquina).
type CategoriaVeiculo string

var TodasCategoriasVeiculo = []CategoriaVeiculo{"LEVE", "UTILITARIO", "EXTRAPESADO"}

func (c CategoriaVeiculo) Valida() bool {
	if c == "" {
		return true
	}
	for _, o := range TodasCategoriasVeiculo {
		if o == c {
			return true
		}
	}
	return false
}

func (c CategoriaVeiculo) Rotulo() string {
	switch c {
	case "LEVE":
		return "Leve"
	case "UTILITARIO":
		return "Utilitário"
	case "EXTRAPESADO":
		return "Extrapesado"
	}
	return string(c)
}

// Exemplos diz o que entra em cada categoria (texto de apoio no formulário).
func (c CategoriaVeiculo) Exemplos() string {
	switch c {
	case "LEVE":
		return "Moto, carro de passeio"
	case "UTILITARIO":
		return "Caminhonete, van, furgão"
	case "EXTRAPESADO":
		return "Caminhão, ônibus, máquina"
	}
	return ""
}

// Icone é o nome do símbolo SVG usado no formulário.
func (c CategoriaVeiculo) Icone() string {
	switch c {
	case "LEVE":
		return "i-carro"
	case "UTILITARIO":
		return "i-caminhonete"
	case "EXTRAPESADO":
		return "i-caminhao"
	}
	return ""
}

// ErroTransicao é devolvido quando se tenta uma mudança de status não permitida.
type ErroTransicao struct {
	De, Para Status
}

func (e *ErroTransicao) Error() string {
	return fmt.Sprintf("não é possível mudar de %s para %s", e.De.Rotulo(), e.Para.Rotulo())
}
