# Exemplo completo: agregado com status e itens

Versão reduzida da Ordem de Serviço para mostrar o encaixe dos arquivos. A OS
real tem mais campos (veículo, trajeto, pagamento, snapshot de cliente) — as
regras completas estão no legado `internal/ordemservico` e no plano.

## `ordemservico.go`

```go
// Package ordemservico é o agregado da Ordem de Serviço: nasce como orçamento
// e vira OS com o mesmo número. Este pacote não sabe de HTTP nem de banco.
package ordemservico

import (
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/domain/guard"
	"github.com/Gustavo-Resende/autoReboque/internal/domain/shared"
)

type OrdemServico struct {
	shared.EntidadeBase
	EmpresaID shared.ID
	Numero    Numero // sequencial por ano (0142/2026); atribuído ao salvar pela primeira vez

	Status      Status
	DataEmissao time.Time
	ValidoAte   time.Time // validade do orçamento; "expirado" é derivado, nunca gravado
	AprovadaEm  time.Time
	ConcluidaEm time.Time

	Cliente          ClienteSnapshot // cópia dos dados na emissão: editar o cadastro não muda OS antigas
	Itens            []Item
	DescontoCentavos shared.Centavos
}

// NovaOrdemServico cria um orçamento aberto, válido por validadeDias.
func NovaOrdemServico(empresaID shared.ID, cliente ClienteSnapshot, validadeDias int, agora time.Time) (*OrdemServico, error) {
	if err := guard.Juntar(
		guard.Que(empresaID != shared.IDVazio, "empresa_id", "obrigatório"),
		guard.NaoVazio("cliente.nome", cliente.Nome),
		guard.Positivo("validade_dias", validadeDias),
	); err != nil {
		return nil, err
	}
	hoje := somenteData(agora)
	return &OrdemServico{
		EntidadeBase: shared.NovaEntidadeBase(agora),
		EmpresaID:    empresaID,
		Status:       OrcamentoAberto,
		DataEmissao:  hoje,
		ValidoAte:    hoje.AddDate(0, 0, validadeDias),
		Cliente:      cliente,
	}, nil
}

// AdicionarItem valida o item e o coloca na última posição.
func (os *OrdemServico) AdicionarItem(item Item, agora time.Time) error {
	if !os.Status.EhOrcamento() && os.Status != EmAndamento {
		return ErrItensBloqueados
	}
	if err := item.validar(); err != nil {
		return err
	}
	item.Posicao = len(os.Itens) + 1
	os.Itens = append(os.Itens, item)
	os.Tocar(agora)
	return nil
}

func (os *OrdemServico) Subtotal() shared.Centavos {
	var total shared.Centavos
	for _, it := range os.Itens {
		total += it.Subtotal()
	}
	return total
}

func (os *OrdemServico) Total() shared.Centavos { return os.Subtotal() - os.DescontoCentavos }

// AplicarDesconto: nunca maior que o subtotal, nunca negativo.
func (os *OrdemServico) AplicarDesconto(valor shared.Centavos, agora time.Time) error {
	if err := guard.Entre("desconto", valor, 0, os.Subtotal()); err != nil {
		return err
	}
	os.DescontoCentavos = valor
	os.Tocar(agora)
	return nil
}

// StatusExibido é o status como a pessoa vê: orçamento vencido aparece expirado.
func (os *OrdemServico) StatusExibido(agora time.Time) Status {
	if (os.Status == OrcamentoAberto || os.Status == OrcamentoEnviado) && os.ValidoAte.Before(somenteData(agora)) {
		return OrcamentoExpirado
	}
	return os.Status
}

// MudarStatus aplica uma transição permitida e grava os carimbos de tempo.
func (os *OrdemServico) MudarStatus(novo Status, validadeDias int, agora time.Time) error {
	atual := os.StatusExibido(agora)
	if !atual.PodeIrPara(novo) {
		return ErrTransicaoInvalida.Comf("não é possível ir de %s para %s", atual, novo)
	}
	switch {
	case atual.EhOrcamento() && !novo.EhOrcamento():
		os.AprovadaEm = agora
	case novo == OrcamentoAberto: // reabrir/renovar
		os.ValidoAte = somenteData(agora).AddDate(0, 0, validadeDias)
	case novo == Concluida:
		os.ConcluidaEm = agora
	}
	os.Status = novo
	os.Tocar(agora)
	return nil
}

func somenteData(t time.Time) time.Time {
	a, m, d := t.Date()
	return time.Date(a, m, d, 0, 0, 0, 0, time.UTC)
}
```

## `item.go`

```go
package ordemservico

type Item struct {
	Posicao              int
	ServicoID            *shared.ID // de onde veio no catálogo; nil = item livre
	Descricao            string
	Unidade              Unidade
	QuantidadeCentesimos shared.Centesimos
	ValorUnitario        shared.Centavos
}

func (i Item) Subtotal() shared.Centavos { return i.QuantidadeCentesimos.Vezes(i.ValorUnitario) }

func (i Item) validar() error {
	return guard.Juntar(
		guard.NaoVazio("descricao", i.Descricao),
		guard.TamanhoMax("descricao", i.Descricao, 200),
		guard.NaoNegativo("quantidade", i.QuantidadeCentesimos),
		guard.NaoNegativo("valor_unitario", i.ValorUnitario),
		guard.UmDe("unidade", i.Unidade, UN, KM, Hora, Diaria),
	)
}
```

## `status.go`

```go
package ordemservico

// Status é o andamento do documento; os valores são os gravados no banco.
// Os três primeiros são a fase de ORÇAMENTO; os demais, a fase de OS.
type Status string

const (
	OrcamentoAberto   Status = "ORCAMENTO_ABERTO"
	OrcamentoEnviado  Status = "ORCAMENTO_ENVIADO"
	OrcamentoRecusado Status = "ORCAMENTO_RECUSADO"
	Agendada          Status = "AGENDADA"
	EmAndamento       Status = "EM_ANDAMENTO"
	Concluida         Status = "CONCLUIDA"
	Cancelada         Status = "CANCELADA"
	// OrcamentoExpirado nunca é gravado: é derivado da validade.
	OrcamentoExpirado Status = "ORCAMENTO_EXPIRADO"
)

// transicoes: de cada status, para quais ele pode ir. Tabela > switch: dá
// para testar exaustivamente e listar as ações na UI.
var transicoes = map[Status][]Status{
	OrcamentoAberto:   {OrcamentoEnviado, Agendada, EmAndamento, OrcamentoRecusado},
	OrcamentoEnviado:  {Agendada, EmAndamento, OrcamentoRecusado},
	OrcamentoRecusado: {OrcamentoAberto},
	OrcamentoExpirado: {OrcamentoAberto, Agendada, EmAndamento, OrcamentoRecusado},
	Agendada:          {EmAndamento, Cancelada},
	EmAndamento:       {Concluida, Cancelada},
	Concluida:         {EmAndamento}, // "reabrir": corrige um toque errado
	Cancelada:         {},
}

func (s Status) Valido() bool { _, ok := transicoes[s]; return ok && s != OrcamentoExpirado }
func (s Status) EhOrcamento() bool {
	return s == OrcamentoAberto || s == OrcamentoEnviado || s == OrcamentoRecusado || s == OrcamentoExpirado
}
func (s Status) Transicoes() []Status { return transicoes[s] }
func (s Status) PodeIrPara(destino Status) bool {
	for _, t := range transicoes[s] {
		if t == destino {
			return true
		}
	}
	return false
}
```

Rótulos para a pessoa ("Em andamento", "Aprovar e agendar") **não** ficam no
domínio: são do frontend. A API devolve o código.

## `erros.go`

```go
package ordemservico

import "github.com/Gustavo-Resende/autoReboque/pkg/erros"

var (
	ErrNaoEncontrada     = erros.NaoEncontrado("os.nao_encontrada", "ordem de serviço não encontrada")
	ErrTransicaoInvalida = erros.Regra("os.transicao_invalida", "mudança de status não permitida")
	ErrItensBloqueados   = erros.Regra("os.itens_bloqueados", "os itens só podem mudar em orçamento ou OS em andamento")
	ErrStatusInvalido    = erros.Validacao("os.status_invalido", "status inválido")
)
```

## `repository.go`

```go
package ordemservico

import (
	"context"

	"github.com/Gustavo-Resende/autoReboque/internal/domain/shared"
)

// Repository é o port de persistência do agregado. A implementação (Postgres)
// fica em internal/adapters/postgres e é injetada nos handlers de aplicação.
type Repository interface {
	// Salvar insere ou atualiza a OS e seus itens numa única unidade.
	// Devolve shared.ErrVersaoDesatualizada se a versão gravada não for a carregada.
	Salvar(ctx context.Context, os *OrdemServico) error
	// ObterPorID devolve ErrNaoEncontrada para id inexistente, de outra empresa ou excluído.
	ObterPorID(ctx context.Context, empresaID, id shared.ID) (*OrdemServico, error)
	// ProximoNumero reserva o próximo número do ano dentro da transação corrente.
	ProximoNumero(ctx context.Context, empresaID shared.ID, ano int) (int, error)
}
```

## `ordemservico_test.go`

```go
package ordemservico_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/domain/ordemservico"
	"github.com/Gustavo-Resende/autoReboque/internal/domain/shared"
)

var agora = time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

func orcamentoAberto(t *testing.T) *ordemservico.OrdemServico {
	t.Helper()
	os, err := ordemservico.NovaOrdemServico(shared.NovoID(), ordemservico.ClienteSnapshot{Nome: "João"}, 3, agora)
	if err != nil {
		t.Fatalf("OS válida não deveria falhar: %v", err)
	}
	return os
}

func TestOrdemServico_NaoConcluiSemIniciar(t *testing.T) {
	os := orcamentoAberto(t)
	err := os.MudarStatus(ordemservico.Concluida, 3, agora)
	if !errors.Is(err, ordemservico.ErrTransicaoInvalida) {
		t.Fatalf("esperava ErrTransicaoInvalida, veio %v", err)
	}
}

func TestOrdemServico_OrcamentoVencidoApareceExpirado(t *testing.T) {
	os := orcamentoAberto(t)
	depois := agora.AddDate(0, 0, 4)
	if got := os.StatusExibido(depois); got != ordemservico.OrcamentoExpirado {
		t.Fatalf("esperava expirado, veio %s", got)
	}
}

func TestOrdemServico_DescontoNaoPassaDoSubtotal(t *testing.T) {
	os := orcamentoAberto(t)
	_ = os.AdicionarItem(ordemservico.Item{Descricao: "Saída", Unidade: ordemservico.UN, QuantidadeCentesimos: 100, ValorUnitario: 15000}, agora)
	if err := os.AplicarDesconto(20000, agora); err == nil {
		t.Fatal("desconto maior que o subtotal deveria falhar")
	}
}
```
