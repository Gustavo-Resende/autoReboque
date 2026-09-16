// Package ordemservico é o domínio da Ordem de Serviço: a entidade, seus
// itens, os eixos de status, a numeração por ano e o repositório.
//
// Este pacote não sabe nada de HTTP nem de HTML. Quem chama é a camada web.
package ordemservico

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// OrigemParticular é a única origem de atendimento por enquanto. O campo existe
// para que, no futuro, atendimentos de seguradoras sejam registrados na mesma tabela.
const OrigemParticular = "PARTICULAR"

// Cliente é a cópia dos dados do cliente no momento da emissão.
type Cliente struct {
	ID        int64
	Nome      string
	Documento string
	Telefone  string
}

// Solicitante é quem pediu o serviço, quando não é o próprio cliente
// (a oficina que chama pelo cliente dela, o filho que chama pelo pai).
type Solicitante struct {
	Nome     string
	Telefone string
}

type Veiculo struct {
	Categoria CategoriaVeiculo
	Modelo    string
	Ano       string
	Cor       string
	Placa     string
	Condicao  string // "roda", "travado", "capotado", "sem chave"...
}

type Trajeto struct {
	Origem     string
	Referencia string // ponto de referência do local
	Destino    string
	Km         int // 0 = não informado
}

// Motorista é o vínculo + cópia do nome, como no cliente.
type Motorista struct {
	ID   int64 // 0 = sem vínculo
	Nome string
}

// Pagamento é o eixo financeiro: valor pago acumulado, forma e vencimento.
type Pagamento struct {
	Status        StatusPagamento
	ValorCentavos int64
	Forma         FormaPagamento
	Vencimento    time.Time // só quando faturado
}

// OS é a Ordem de Serviço. Nasce como orçamento e é o mesmo registro, com o
// mesmo número, durante toda a vida do serviço.
type OS struct {
	ID     int64
	Numero Numero

	// Versao cresce a cada gravação. Quem edita manda a versão que carregou;
	// se o banco já estiver em outra, a gravação é recusada (optimistic locking).
	Versao int

	OrigemAtendimento string
	Status            Status
	DataEmissao       time.Time // só a data importa
	ValidoAte         time.Time // validade do orçamento (zero = sem validade)
	AgendadaPara      time.Time
	AcionadoEm        time.Time
	AprovadaEm        time.Time
	ConcluidaEm       time.Time

	Cliente     Cliente
	Solicitante Solicitante
	Veiculo     Veiculo
	Trajeto     Trajeto
	Motorista   Motorista
	Guincho     string
	RecebidoPor string
	Observacoes string

	Itens            []Item
	DescontoCentavos int64
	Pagamento        Pagamento

	CriadaEm     time.Time
	AtualizadaEm time.Time
}

// Nova devolve uma OS com os padrões de nascimento: orçamento aberto, pendente,
// hoje, válida por validadeDias.
func Nova(validadeDias int) *OS {
	os := &OS{
		OrigemAtendimento: OrigemParticular,
		Status:            OrcamentoAberto,
		DataEmissao:       hoje(),
		Pagamento:         Pagamento{Status: Pendente},
	}
	os.RenovarValidade(validadeDias)
	return os
}

func hoje() time.Time {
	ano, mes, dia := time.Now().Date()
	return time.Date(ano, mes, dia, 0, 0, 0, 0, time.UTC)
}

// RenovarValidade conta a validade a partir de hoje.
func (os *OS) RenovarValidade(dias int) {
	if dias < 1 {
		dias = 1
	}
	os.ValidoAte = hoje().AddDate(0, 0, dias)
}

// Subtotal é a soma dos itens, antes do desconto.
func (os *OS) Subtotal() int64 {
	return SomarItens(os.Itens)
}

// Total é subtotal menos desconto, em centavos.
func (os *OS) Total() int64 {
	return os.Subtotal() - os.DescontoCentavos
}

// SaldoCentavos é quanto falta pagar (pode ser negativo se pagou a mais).
func (os *OS) SaldoCentavos() int64 {
	return os.Total() - os.Pagamento.ValorCentavos
}

// EhOrcamento diz se o documento ainda está na fase de orçamento.
func (os *OS) EhOrcamento() bool {
	return os.Status.EhOrcamento()
}

// Expirado diz se é um orçamento aberto/enviado cuja validade já passou.
func (os *OS) Expirado() bool {
	if os.Status != OrcamentoAberto && os.Status != OrcamentoEnviado {
		return false
	}
	return !os.ValidoAte.IsZero() && os.ValidoAte.Before(hoje())
}

// StatusExibido é o status como a pessoa vê: igual ao gravado, exceto que um
// orçamento vencido aparece como expirado.
func (os *OS) StatusExibido() Status {
	if os.Expirado() {
		return OrcamentoExpirado
	}
	return os.Status
}

// Titulo é o nome do documento impresso, que acompanha a fase.
func (os *OS) Titulo() string {
	if os.EhOrcamento() {
		return "ORÇAMENTO"
	}
	return "ORDEM DE SERVIÇO"
}

// Acoes lista os status para os quais a OS pode ir agora.
func (os *OS) Acoes() []Status {
	return os.StatusExibido().Transicoes()
}

// MudarStatus aplica uma transição, se permitida a partir do status exibido.
// Registra os carimbos de tempo que fazem sentido em cada passagem.
// A validade do orçamento é renovada ao reabrir/renovar (validadeDias).
func (os *OS) MudarStatus(novo Status, validadeDias int) error {
	if !novo.Valido() {
		return fmt.Errorf("status inválido: %q", novo)
	}
	atual := os.StatusExibido()
	if !atual.PodeIrPara(novo) {
		return &ErroTransicao{De: atual, Para: novo}
	}
	agora := time.Now()
	switch {
	case atual.EhOrcamento() && !novo.EhOrcamento():
		os.AprovadaEm = agora
	case novo == OrcamentoAberto:
		os.RenovarValidade(validadeDias)
	case novo == Concluida:
		os.ConcluidaEm = agora
	}
	if novo != Agendada {
		os.AgendadaPara = time.Time{}
	}
	os.Status = novo
	return nil
}

// RegistrarPagamento atualiza o eixo financeiro. É só um valor pago acumulado,
// a forma, o vencimento (quando faturado) e o status escolhido pela pessoa.
func (os *OS) RegistrarPagamento(p Pagamento) error {
	if !p.Status.Valido() {
		return fmt.Errorf("status de pagamento inválido: %q", p.Status)
	}
	if !p.Forma.Valida() {
		return fmt.Errorf("forma de pagamento inválida: %q", p.Forma)
	}
	if p.ValorCentavos < 0 {
		return errors.New("valor pago não pode ser negativo")
	}
	if p.Forma != Faturado {
		p.Vencimento = time.Time{}
	}
	os.Pagamento = p
	return nil
}

// ErroValidacao junta todas as mensagens de um Validar que falhou.
type ErroValidacao struct {
	Mensagens []string
}

func (e *ErroValidacao) Error() string {
	return strings.Join(e.Mensagens, "; ")
}

// Validar confere as regras que não dependem do banco. Só número, data,
// status e cliente são obrigatórios; o resto é preenchido conforme fizer sentido.
func (os *OS) Validar() error {
	var msgs []string
	if os.DataEmissao.IsZero() {
		msgs = append(msgs, "data de emissão é obrigatória")
	}
	if !os.Status.Valido() {
		msgs = append(msgs, "status inválido")
	}
	if !os.Pagamento.Status.Valido() {
		msgs = append(msgs, "status de pagamento inválido")
	}
	if !os.Pagamento.Forma.Valida() {
		msgs = append(msgs, "forma de pagamento inválida")
	}
	if os.OrigemAtendimento == "" {
		msgs = append(msgs, "origem do atendimento é obrigatória")
	}
	if os.Cliente.ID == 0 {
		msgs = append(msgs, "escolha um cliente")
	}
	if !os.Veiculo.Categoria.Valida() {
		msgs = append(msgs, "categoria do veículo inválida")
	}
	if os.Status == Agendada && os.AgendadaPara.IsZero() {
		msgs = append(msgs, "informe a data e hora do agendamento")
	}
	if os.Trajeto.Km < 0 {
		msgs = append(msgs, "km não pode ser negativo")
	}
	if os.Pagamento.ValorCentavos < 0 {
		msgs = append(msgs, "valor pago não pode ser negativo")
	}
	if os.DescontoCentavos < 0 {
		msgs = append(msgs, "desconto não pode ser negativo")
	}
	for i, it := range os.Itens {
		if strings.TrimSpace(it.Descricao) == "" {
			msgs = append(msgs, fmt.Sprintf("item %d: descrição é obrigatória", i+1))
		}
		if it.QuantidadeCentesimos < 0 {
			msgs = append(msgs, fmt.Sprintf("item %d: quantidade não pode ser negativa", i+1))
		}
		if it.ValorUnitarioCentavos < 0 {
			msgs = append(msgs, fmt.Sprintf("item %d: valor não pode ser negativo (use o campo desconto)", i+1))
		}
	}
	if os.Total() < 0 {
		msgs = append(msgs, "o desconto não pode ser maior que o subtotal")
	}
	if len(msgs) > 0 {
		return &ErroValidacao{Mensagens: msgs}
	}
	return nil
}

// RemoverItensVazios descarta linhas em branco vindas do formulário.
func (os *OS) RemoverItensVazios() {
	itens := os.Itens[:0]
	for _, it := range os.Itens {
		if !it.Vazio() {
			itens = append(itens, it)
		}
	}
	os.Itens = itens
}
