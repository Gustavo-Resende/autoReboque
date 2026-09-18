---
name: criar-caso-de-uso
description: Cria um caso de uso na camada de aplicação (internal/application) no padrão CQRS do autoReboque — Command ou Query com handler tipado, registro no Dispatcher, ports (TxManager, Clock), DTOs de leitura e testes com fakes. Use sempre que o pedido envolver "caso de uso", "command", "query", "handler de aplicação", "dispatcher", "orquestrar", "listar/obter X" (read model) ou "criar/atualizar/mudar X" pela API — inclusive quando o usuário só diz "faz o endpoint de criar OS", porque o endpoint precisa do caso de uso antes.
---

# Criar caso de uso (CQRS)

A camada de aplicação orquestra: carrega agregados pelos ports, chama o
domínio, salva. **Command** escreve, **Query** lê. Sem HTTP, sem SQL, sem regra
de negócio (isso é do domínio). O contrato do pacote `cqrs` está em
[references/cqrs.md](references/cqrs.md) — leia se precisar tocar no
dispatcher ou nos decorators.

## Command ou Query?

| | Command | Query |
|---|---|---|
| Faz | muda estado | só lê |
| Devolve | `shared.ID` do que criou, ou nada (`struct{}`) | DTO de leitura (struct simples) |
| Passa por | recover → log → validação → **transação** | recover → log → validação |
| Fala com | `Repository` do domínio (carrega/salva agregado) | interface de leitura própria (`Leitor`), com SQL otimizado |
| Onde | `application/<modulo>/command/` | `application/<modulo>/query/` |

Se um caso de uso "cria e devolve tudo", são dois: um command que devolve o
ID e uma query que o frontend chama em seguida. Isso mantém a escrita
auditável e a leitura livre para ser otimizada.

## Arquivos

```
internal/application/<modulo>/
  command/
    criar_ordem_servico.go        CriarOrdemServicoCommand + CriarOrdemServicoHandler
    criar_ordem_servico_test.go   testes com fakes
  query/
    obter_ordem_servico.go        ObterOrdemServicoQuery + DTO OrdemServicoDetalhe + interface Leitor
    listar_ordens_servico.go      filtros, paginação, DTO de linha
    query_test.go
internal/application/port/        TxManager, Clock, PasswordHasher… (só interfaces)
```

Um arquivo por caso de uso: request, handler e resultado juntos. Fica fácil
achar tudo de "criar OS" num lugar só.

## Command — modelo

```go
package command

// CriarOrdemServicoCommand abre um orçamento para um cliente já cadastrado.
// Campos são dados crus (strings, ints): a conversão para o domínio acontece no
// handler, e a validação de forma em Validar (a de regra fica no domínio).
type CriarOrdemServicoCommand struct {
	cqrs.Command[shared.ID]

	EmpresaID  shared.ID
	ClienteID  shared.ID
	Itens      []ItemInput
	Observacao string
}

// Validar checa forma e obrigatoriedade antes do handler rodar (decorator de
// validação). Nada de regra de negócio aqui.
func (c CriarOrdemServicoCommand) Validar() error {
	return validar.Juntar(
		validar.UUID("empresa_id", c.EmpresaID),
		validar.UUID("cliente_id", c.ClienteID),
		validar.TamanhoMax("observacao", c.Observacao, 2000),
	)
}

type CriarOrdemServicoHandler struct {
	ordens   ordemservico.Repository
	clientes cliente.Repository
	empresas empresa.Repository
	clock    port.Clock
}

func NovoCriarOrdemServicoHandler(ordens ordemservico.Repository, clientes cliente.Repository, empresas empresa.Repository, clock port.Clock) *CriarOrdemServicoHandler {
	return &CriarOrdemServicoHandler{ordens: ordens, clientes: clientes, empresas: empresas, clock: clock}
}

func (h *CriarOrdemServicoHandler) Handle(ctx context.Context, cmd CriarOrdemServicoCommand) (shared.ID, error) {
	agora := h.clock.Agora()

	cli, err := h.clientes.ObterPorID(ctx, cmd.EmpresaID, cmd.ClienteID)
	if err != nil {
		return shared.IDVazio, err // já é erros.NaoEncontrado: o HTTP vira 404 sozinho
	}
	emp, err := h.empresas.ObterPorID(ctx, cmd.EmpresaID)
	if err != nil {
		return shared.IDVazio, err
	}

	os, err := ordemservico.NovaOrdemServico(cmd.EmpresaID, cli.Snapshot(), emp.ValidadeOrcamentoDias, agora)
	if err != nil {
		return shared.IDVazio, err
	}
	for _, it := range cmd.Itens {
		if err := os.AdicionarItem(it.paraDominio(), agora); err != nil {
			return shared.IDVazio, err
		}
	}

	numero, err := h.ordens.ProximoNumero(ctx, cmd.EmpresaID, agora.Year()) // dentro da transação do decorator
	if err != nil {
		return shared.IDVazio, fmt.Errorf("reservar número: %w", err)
	}
	os.Numero = ordemservico.Numero{Ano: agora.Year(), Sequencial: numero}

	if err := h.ordens.Salvar(ctx, os); err != nil {
		return shared.IDVazio, fmt.Errorf("salvar OS: %w", err)
	}
	return os.ID, nil
}
```

O que faz este handler ser "magro": cada linha é carregar, chamar o domínio
ou salvar. Se aparecer um cálculo ou um `if` de estado, mova para um método do
agregado.

**Transação:** o decorator `Transacao` abre uma `tx` antes do `Handle` de
todo command e a coloca no `ctx`; os repositórios Postgres a encontram via
`context`. O handler nunca chama `Begin/Commit`. Erro → rollback.

## Query — modelo

```go
package query

type ObterOrdemServicoQuery struct {
	cqrs.Query[OrdemServicoDetalhe]
	EmpresaID shared.ID
	ID        shared.ID
}

// OrdemServicoDetalhe é o que a API devolve. É um DTO: sem métodos de negócio,
// tags json aqui mesmo, moldado para a tela — não é a entidade.
type OrdemServicoDetalhe struct {
	ID          shared.ID       `json:"id"`
	Numero      string          `json:"numero"` // "0142/2026"
	Status      string          `json:"status"`
	Cliente     ClienteResumo   `json:"cliente"`
	Itens       []ItemDetalhe   `json:"itens"`
	TotalCentavos int64         `json:"total_centavos"`
	Versao      int             `json:"versao"`
}

// Leitor é o port de leitura deste módulo. Implementado em adapters/postgres
// com SQL feito para a tela (joins, agregações), sem passar pelo agregado.
type Leitor interface {
	ObterDetalhe(ctx context.Context, empresaID, id shared.ID) (OrdemServicoDetalhe, error) // ordemservico.ErrNaoEncontrada
	Listar(ctx context.Context, f FiltroListagem) (Pagina[OrdemServicoLinha], error)
}

type ObterOrdemServicoHandler struct{ leitor Leitor }

func NovoObterOrdemServicoHandler(l Leitor) *ObterOrdemServicoHandler { return &ObterOrdemServicoHandler{leitor: l} }

func (h *ObterOrdemServicoHandler) Handle(ctx context.Context, q ObterOrdemServicoQuery) (OrdemServicoDetalhe, error) {
	return h.leitor.ObterDetalhe(ctx, q.EmpresaID, q.ID)
}
```

Listagens sempre recebem `FiltroListagem` com paginação explícita (`Pagina`,
`PorPagina` limitado a 100) e devolvem `Pagina[T]{Itens, Total, Pagina,
PorPagina}`. Sem isso o frontend faz scroll infinito no banco.

## Ports em `application/port`

Interfaces pequenas para o que não é repositório. Só o que a aplicação
realmente chama:

```go
type Clock interface{ Agora() time.Time }
type TxManager interface{ Executar(ctx context.Context, fn func(ctx context.Context) error) error }
type PasswordHasher interface {
	Hash(senha string) (string, error)
	Confere(hash, senha string) bool
}
```

Implementações reais em `adapters/` (Postgres para `TxManager`, bcrypt em
`adapters/seguranca`), fakes nos testes.

## Registro (composição em `cmd/api/main.go`)

```go
d := cqrs.NewDispatcher(cqrs.Recover(log), cqrs.Log(log), cqrs.Validacao(), cqrs.Transacao(tx))
cqrs.Register(d, command.NovoCriarOrdemServicoHandler(ordens, clientes, empresas, clock))
cqrs.Register(d, query.NovoObterOrdemServicoHandler(leitorOS))
```

`Register` dá `panic` na subida se dois handlers atendem o mesmo request —
melhor quebrar no boot do que em produção.

## Testes

Fakes escritos à mão no `_test.go` (não gerar mocks), guardando o que
receberam para asserção:

```go
type fakeOrdens struct {
	salvas   []*ordemservico.OrdemServico
	proximo  int
}
func (f *fakeOrdens) Salvar(_ context.Context, os *ordemservico.OrdemServico) error { f.salvas = append(f.salvas, os); return nil }
func (f *fakeOrdens) ObterPorID(context.Context, shared.ID, shared.ID) (*ordemservico.OrdemServico, error) { return nil, ordemservico.ErrNaoEncontrada }
func (f *fakeOrdens) ProximoNumero(context.Context, shared.ID, int) (int, error) { f.proximo++; return f.proximo, nil }

type clockFixo time.Time
func (c clockFixo) Agora() time.Time { return time.Time(c) }

func TestCriarOrdemServico_FalhaSeClienteNaoExiste(t *testing.T) {
	h := command.NovoCriarOrdemServicoHandler(&fakeOrdens{}, &fakeClientes{}, &fakeEmpresas{}, clockFixo(agora))
	_, err := h.Handle(context.Background(), command.CriarOrdemServicoCommand{EmpresaID: e, ClienteID: shared.NovoID()})
	if !errors.Is(err, cliente.ErrNaoEncontrado) {
		t.Fatalf("esperava ErrNaoEncontrado, veio %v", err)
	}
}
```

Teste o handler direto (sem dispatcher): o que importa é a orquestração —
que carregou o que devia, que salvou, que propagou o erro do domínio. O
dispatcher tem os próprios testes em `cqrs`.

## Checklist

- [ ] Struct embute `cqrs.Command[R]` ou `cqrs.Query[R]` com `R` certo (ID/`struct{}` para command; DTO para query)
- [ ] `Validar()` só de forma (obrigatório, tamanho, formato); regra de negócio está no domínio
- [ ] Handler não abre transação, não chama `time.Now()`, não tem `if` de regra
- [ ] Query devolve DTO com tags json, nunca entidade; listagem paginada
- [ ] Erros do domínio/repositório propagados sem re-embrulhar em erro genérico (o `Kind` precisa chegar ao HTTP); contexto com `%w` quando ajuda o log
- [ ] Registrado em `cmd/api/main.go`
- [ ] `go list -deps ./internal/application/... | grep -E "adapters|pgx|chi|net/http"` vazio
- [ ] Testes com fakes cobrem caminho feliz e cada erro propagado
- [ ] Commit: `feat(aplicacao): command CriarOrdemServico` (ver `/commits`)
