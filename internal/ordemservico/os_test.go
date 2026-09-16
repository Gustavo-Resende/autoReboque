package ordemservico

import (
	"errors"
	"testing"
	"time"
)

const validadeTeste = 3

// novaComCliente evita repetir o cliente obrigatório em todo teste.
func novaComCliente() *OS {
	os := Nova(validadeTeste)
	os.Cliente = Cliente{ID: 1, Nome: "Serviço particular"}
	return os
}

func TestItemSubtotal(t *testing.T) {
	casos := []struct {
		nome     string
		item     Item
		esperado int64
	}{
		{"inteiro simples", Item{QuantidadeCentesimos: 100, ValorUnitarioCentavos: 15000}, 15000},
		{"km rodada", Item{QuantidadeCentesimos: 3700, ValorUnitarioCentavos: 450}, 16650}, // 37 × 4,50
		{"hora e meia", Item{QuantidadeCentesimos: 150, ValorUnitarioCentavos: 8000}, 12000},
		{"arredonda para baixo", Item{QuantidadeCentesimos: 33, ValorUnitarioCentavos: 1001}, 330}, // 3,3033 → 3,30
		{"arredonda meio para cima", Item{QuantidadeCentesimos: 50, ValorUnitarioCentavos: 1}, 1},  // 0,005 → 0,01
		{"quantidade zero", Item{QuantidadeCentesimos: 0, ValorUnitarioCentavos: 9999}, 0},
	}
	for _, c := range casos {
		if got := c.item.Subtotal(); got != c.esperado {
			t.Errorf("%s: subtotal = %d, esperado %d", c.nome, got, c.esperado)
		}
	}
}

func TestTotalComDesconto(t *testing.T) {
	os := novaComCliente()
	os.Itens = []Item{
		{Descricao: "Saída de base", QuantidadeCentesimos: 100, ValorUnitarioCentavos: 15000},
		{Descricao: "Quilometragem rodada", QuantidadeCentesimos: 3700, ValorUnitarioCentavos: 450},
		{Descricao: "Pedágio", QuantidadeCentesimos: 200, ValorUnitarioCentavos: 1230},
	}
	os.DescontoCentavos = 2000
	// 150,00 + 166,50 + 24,60 = 341,10; - 20,00 = 321,10
	if got := os.Subtotal(); got != 34110 {
		t.Errorf("subtotal = %d, esperado 34110", got)
	}
	if got := os.Total(); got != 32110 {
		t.Errorf("total = %d, esperado 32110", got)
	}
	os.Pagamento.ValorCentavos = 10000
	if got := os.SaldoCentavos(); got != 22110 {
		t.Errorf("saldo = %d, esperado 22110", got)
	}

	os.DescontoCentavos = 99999
	if err := os.Validar(); err == nil {
		t.Error("desconto maior que o subtotal deveria ser inválido")
	}
	os.DescontoCentavos = 0
	os.Itens[0].ValorUnitarioCentavos = -1
	if err := os.Validar(); err == nil {
		t.Error("item com valor negativo deveria ser inválido (desconto é campo próprio)")
	}
}

func TestTransicoesDeStatus(t *testing.T) {
	permitidas := []struct{ de, para Status }{
		{OrcamentoAberto, OrcamentoEnviado},
		{OrcamentoAberto, Agendada},
		{OrcamentoAberto, EmAndamento},
		{OrcamentoAberto, OrcamentoRecusado},
		{OrcamentoEnviado, EmAndamento},
		{OrcamentoEnviado, OrcamentoRecusado},
		{OrcamentoRecusado, OrcamentoAberto},
		{Agendada, EmAndamento},
		{Agendada, Cancelada},
		{EmAndamento, Concluida},
		{EmAndamento, Cancelada},
		{Concluida, EmAndamento},
	}
	for _, c := range permitidas {
		os := novaComCliente()
		os.Status = c.de
		if c.para == Agendada {
			os.AgendadaPara = time.Now().Add(24 * time.Hour)
		}
		if err := os.MudarStatus(c.para, validadeTeste); err != nil {
			t.Errorf("%s → %s deveria ser permitida: %v", c.de, c.para, err)
		}
		if os.Status != c.para {
			t.Errorf("%s → %s: status ficou %s", c.de, c.para, os.Status)
		}
	}

	proibidas := []struct{ de, para Status }{
		{OrcamentoAberto, Concluida},
		{OrcamentoAberto, Cancelada},
		{OrcamentoAberto, OrcamentoAberto},
		{OrcamentoEnviado, OrcamentoEnviado},
		{OrcamentoRecusado, EmAndamento},
		{EmAndamento, OrcamentoAberto},
		{Concluida, Cancelada},
		{Cancelada, OrcamentoAberto},
		{Cancelada, EmAndamento},
	}
	for _, c := range proibidas {
		os := novaComCliente()
		os.Status = c.de
		err := os.MudarStatus(c.para, validadeTeste)
		var errTransicao *ErroTransicao
		if !errors.As(err, &errTransicao) {
			t.Errorf("%s → %s deveria ser recusada com ErroTransicao, veio %v", c.de, c.para, err)
		}
		if os.Status != c.de {
			t.Errorf("%s → %s recusada mas status mudou para %s", c.de, c.para, os.Status)
		}
	}

	if err := novaComCliente().MudarStatus("QUALQUER_COISA", validadeTeste); err == nil {
		t.Error("status desconhecido deveria ser recusado")
	}
	if err := novaComCliente().MudarStatus(OrcamentoExpirado, validadeTeste); err == nil {
		t.Error("expirado é virtual: não pode ser gravado")
	}
}

func TestAprovacaoRegistraCarimbos(t *testing.T) {
	os := novaComCliente()
	if err := os.MudarStatus(EmAndamento, validadeTeste); err != nil {
		t.Fatal(err)
	}
	if os.AprovadaEm.IsZero() {
		t.Error("aprovar deveria registrar AprovadaEm")
	}
	if err := os.MudarStatus(Concluida, validadeTeste); err != nil {
		t.Fatal(err)
	}
	if os.ConcluidaEm.IsZero() {
		t.Error("concluir deveria registrar ConcluidaEm")
	}
}

func TestOrcamentoExpira(t *testing.T) {
	os := novaComCliente()
	if os.Expirado() || os.StatusExibido() != OrcamentoAberto {
		t.Fatalf("orçamento novo não deveria estar expirado: %s", os.StatusExibido())
	}
	os.ValidoAte = hoje().AddDate(0, 0, -1)
	if !os.Expirado() || os.StatusExibido() != OrcamentoExpirado {
		t.Fatalf("validade ontem deveria expirar: %s", os.StatusExibido())
	}

	// Expirado ainda pode ser aprovado, e "renovar" volta a valer.
	renovado := *os
	if err := renovado.MudarStatus(OrcamentoAberto, 5); err != nil {
		t.Fatalf("renovar deveria ser permitido: %v", err)
	}
	if renovado.Expirado() || !renovado.ValidoAte.Equal(hoje().AddDate(0, 0, 5)) {
		t.Errorf("renovar deveria estender a validade: %v", renovado.ValidoAte)
	}
	if err := os.MudarStatus(EmAndamento, validadeTeste); err != nil {
		t.Errorf("aprovar um expirado deveria ser permitido: %v", err)
	}

	// Depois de aprovado, a validade não importa mais.
	if os.Expirado() {
		t.Error("OS aprovada não expira")
	}
}

func TestTituloAcompanhaFase(t *testing.T) {
	os := novaComCliente()
	for _, s := range []Status{OrcamentoAberto, OrcamentoEnviado, OrcamentoRecusado} {
		os.Status = s
		if os.Titulo() != "ORÇAMENTO" {
			t.Errorf("%s: título = %q", s, os.Titulo())
		}
	}
	for _, s := range []Status{Agendada, EmAndamento, Concluida, Cancelada} {
		os.Status = s
		if os.Titulo() != "ORDEM DE SERVIÇO" {
			t.Errorf("%s: título = %q", s, os.Titulo())
		}
	}
}

func TestRegistrarPagamento(t *testing.T) {
	os := novaComCliente()
	venc := hoje().AddDate(0, 0, 30)
	if err := os.RegistrarPagamento(Pagamento{Status: Parcial, ValorCentavos: 5000, Forma: Faturado, Vencimento: venc}); err != nil {
		t.Fatal(err)
	}
	if os.Pagamento.Status != Parcial || os.Pagamento.ValorCentavos != 5000 || os.Pagamento.Forma != Faturado || !os.Pagamento.Vencimento.Equal(venc) {
		t.Errorf("pagamento não registrado como esperado: %+v", os.Pagamento)
	}

	// Vencimento só faz sentido quando faturado.
	if err := os.RegistrarPagamento(Pagamento{Status: Paga, ValorCentavos: 5000, Forma: PIX, Vencimento: venc}); err != nil {
		t.Fatal(err)
	}
	if !os.Pagamento.Vencimento.IsZero() {
		t.Error("PIX não deveria guardar vencimento")
	}

	if err := os.RegistrarPagamento(Pagamento{Status: Paga, ValorCentavos: -1}); err == nil {
		t.Error("valor negativo deveria ser recusado")
	}
	if err := os.RegistrarPagamento(Pagamento{Status: "QUITADA"}); err == nil {
		t.Error("status desconhecido deveria ser recusado")
	}
	if err := os.RegistrarPagamento(Pagamento{Status: Paga, Forma: "CHEQUE"}); err == nil {
		t.Error("forma desconhecida deveria ser recusada")
	}
}

func TestValidar(t *testing.T) {
	os := novaComCliente()
	if err := os.Validar(); err != nil {
		t.Errorf("OS nova com cliente deveria ser válida: %v", err)
	}

	semCliente := Nova(validadeTeste)
	if err := semCliente.Validar(); err == nil {
		t.Error("OS sem cliente deveria ser inválida")
	}

	os.Itens = []Item{{Descricao: "", QuantidadeCentesimos: 100, ValorUnitarioCentavos: 100}}
	if err := os.Validar(); err == nil {
		t.Error("item sem descrição deveria ser inválido")
	}
	os.Itens = nil

	os.Status = Agendada
	if err := os.Validar(); err == nil {
		t.Error("agendada sem data deveria ser inválida")
	}
}

func TestRemoverItensVazios(t *testing.T) {
	os := novaComCliente()
	os.Itens = []Item{
		{},
		{Descricao: "Pedágio", QuantidadeCentesimos: 100, ValorUnitarioCentavos: 1230},
		{},
	}
	os.RemoverItensVazios()
	if len(os.Itens) != 1 || os.Itens[0].Descricao != "Pedágio" {
		t.Errorf("itens após remoção: %+v", os.Itens)
	}
}
