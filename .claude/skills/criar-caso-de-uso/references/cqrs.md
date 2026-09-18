# `internal/application/cqrs` — contrato e implementação de referência

Validado em Go 1.27: a inferência de tipos funciona (`cqrs.Send(ctx, d, req)`
devolve `R` sem cast). Reflexão só no registro/despacho (`reflect.TypeOf`),
nunca dentro de handlers. Sem biblioteca externa.

## Por que assim

- **Um ponto de entrada** para todo caso de uso: HTTP (e no futuro CLI, jobs)
  chama `Send`; decorators (log, validação, transação) valem para todos sem
  repetir código.
- **Tipado**: o compilador garante que `CriarOrdemServicoCommand` devolve
  `shared.ID`. O `R` no marcador `Command[R]` é o que permite isso.
- **Command vs Query é declarado no tipo**, então o decorator de transação
  sabe quando abrir tx sem olhar nomes.

## Implementação

```go
package cqrs

import (
	"context"
	"fmt"
	"reflect"
)

type kind int

const (
	kindCommand kind = iota + 1
	kindQuery
)

// Request é o que se envia ao Dispatcher. R é o tipo do resultado.
// Os métodos são não exportados de propósito: só se implementa embutindo
// Command[R] ou Query[R].
type Request[R any] interface {
	resultado() R
	tipo() kind
}

// Command marca uma escrita. Roda dentro de transação.
type Command[R any] struct{}

func (Command[R]) resultado() (r R) { return }
func (Command[R]) tipo() kind       { return kindCommand }

// Query marca uma leitura. Sem transação.
type Query[R any] struct{}

func (Query[R]) resultado() (r R) { return }
func (Query[R]) tipo() kind       { return kindQuery }

// Handler executa um request específico.
type Handler[Req Request[R], R any] interface {
	Handle(ctx context.Context, req Req) (R, error)
}

// Info descreve o request para os decorators (log, transação).
type Info struct {
	Nome      string // "command.CriarOrdemServicoCommand"
	EhCommand bool
}

// Next é a continuação da cadeia; Decorator envolve uma Next.
type Next func(ctx context.Context, req any) (any, error)
type Decorator func(info Info, next Next) Next

type Dispatcher struct {
	handlers   map[reflect.Type]Next
	decorators []Decorator // aplicados na ordem dada: o primeiro é o mais externo
}

func NewDispatcher(decorators ...Decorator) *Dispatcher {
	return &Dispatcher{handlers: map[reflect.Type]Next{}, decorators: decorators}
}

// Register liga o tipo do request ao handler. Chamado só na composição
// (cmd/api/main.go); duplicidade é bug de wiring, por isso panic.
func Register[Req Request[R], R any](d *Dispatcher, h Handler[Req, R]) {
	var zero Req
	t := reflect.TypeOf(zero)
	if _, dup := d.handlers[t]; dup {
		panic(fmt.Sprintf("cqrs: handler duplicado para %s", t))
	}
	next := Next(func(ctx context.Context, req any) (any, error) {
		return h.Handle(ctx, req.(Req))
	})
	info := Info{Nome: t.String(), EhCommand: zero.tipo() == kindCommand}
	for i := len(d.decorators) - 1; i >= 0; i-- {
		next = d.decorators[i](info, next)
	}
	d.handlers[t] = next
}

// Send despacha o request e devolve o resultado tipado.
func Send[R any](ctx context.Context, d *Dispatcher, req Request[R]) (R, error) {
	var zero R
	next, ok := d.handlers[reflect.TypeOf(req)]
	if !ok {
		return zero, fmt.Errorf("cqrs: nenhum handler registrado para %T", req)
	}
	res, err := next(ctx, req)
	if err != nil {
		return zero, err
	}
	return res.(R), nil
}
```

## Decorators (arquivo `decorators.go`)

Ordem de uso: `Recover, Log, Validacao, Transacao`. O mais externo primeiro.

```go
// Validavel é opcional: requests que o implementam são validados antes do handler.
type Validavel interface{ Validar() error }

func Validacao() Decorator {
	return func(info Info, next Next) Next {
		return func(ctx context.Context, req any) (any, error) {
			if v, ok := req.(Validavel); ok {
				if err := v.Validar(); err != nil {
					return nil, err // *erros.Erro de Validacao: vira 400 com os campos
				}
			}
			return next(ctx, req)
		}
	}
}

// Transacao envolve só commands. O TxManager coloca a tx no ctx; os
// repositórios Postgres a recuperam com postgres.QuerierDe(ctx).
func Transacao(tx port.TxManager) Decorator {
	return func(info Info, next Next) Next {
		if !info.EhCommand {
			return next
		}
		return func(ctx context.Context, req any) (any, error) {
			var res any
			err := tx.Executar(ctx, func(ctx context.Context) error {
				var err error
				res, err = next(ctx, req)
				return err
			})
			return res, err
		}
	}
}

func Log(log *slog.Logger) Decorator {
	return func(info Info, next Next) Next {
		return func(ctx context.Context, req any) (any, error) {
			inicio := time.Now()
			res, err := next(ctx, req)
			nivel := slog.LevelInfo
			if err != nil && erros.KindDe(err) == erros.Interno {
				nivel = slog.LevelError
			}
			log.Log(ctx, nivel, "caso de uso", "nome", info.Nome, "duracao", time.Since(inicio), "erro", err)
			return res, err
		}
	}
}

func Recover(log *slog.Logger) Decorator {
	return func(info Info, next Next) Next {
		return func(ctx context.Context, req any) (res any, err error) {
			defer func() {
				if p := recover(); p != nil {
					log.ErrorContext(ctx, "panic em caso de uso", "nome", info.Nome, "panic", p, "stack", string(debug.Stack()))
					err = erros.Interno(fmt.Errorf("panic: %v", p))
				}
			}()
			return next(ctx, req)
		}
	}
}
```

## Testes do pacote

- `Send` sem handler registrado devolve erro claro.
- `Register` duplicado dá panic.
- `Transacao` abre tx para command e **não** abre para query (fake TxManager
  que conta chamadas).
- `Validacao` bloqueia request com `Validar()` falhando e não chama o handler.
- Um decorator recebe `Info.EhCommand` correto.
