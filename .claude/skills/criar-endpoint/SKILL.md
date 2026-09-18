---
name: criar-endpoint
description: Cria ou altera rotas HTTP em internal/adapters/http no padrão do autoReboque — rota no chi, handler magro (decodificar + validar → cqrs.Send → responder), DTOs de request/response com validação de entrada via pkg/validar, mapeamento único de erros para status HTTP, middlewares (auth, CORS, log) e testes com httptest. Use sempre que o pedido envolver "endpoint", "rota", "handler HTTP", "API", "REST", "JSON", "request/response", "middleware", "CORS", "status code", "validação de entrada" ou "expor X para o frontend".
---

# Criar endpoint (adapter HTTP)

O adapter HTTP é a porta de entrada: traduz HTTP ↔ casos de uso. Ele **não
decide nada de negócio**. A prova de que está certo: dá para trocar por gRPC
ou CLI sem tocar em `application` nem `domain`.

Contratos de `pkg/httpx` e o formato de erro estão em
[references/httpx.md](references/httpx.md).

## Arquivos

```
internal/adapters/http/
  router.go                 NovoRouter(deps) → chi.Router: middlewares globais + montagem das rotas por módulo
  middleware/
    autenticacao.go         Bearer opaco → sessão no ctx (401 se inválido)
    cors.go  requestid.go  log.go  recover.go
  handler/
    ordemservico.go         OrdemServicoHandler{d *cqrs.Dispatcher}: Rotas(r chi.Router) + um método por endpoint
    ordemservico_test.go
  dto/
    ordemservico.go         CriarOrdemServicoRequest{…} Validar() ParaCommand(...) · respostas quando não são o DTO da query
```

Um handler por módulo, com um método `Rotas` que registra o grupo. O
`router.go` só chama `handler.Rotas`, então a lista de rotas do sistema cabe
numa tela.

## O handler — sempre no mesmo formato

```go
type OrdemServicoHandler struct{ d *cqrs.Dispatcher }

func (h *OrdemServicoHandler) Rotas(r chi.Router) {
	r.Post("/ordens-servico", h.Criar)
	r.Get("/ordens-servico", h.Listar)
	r.Get("/ordens-servico/{id}", h.Obter)
	r.Post("/ordens-servico/{id}/status", h.MudarStatus)
}

// Criar: POST /api/v1/ordens-servico → 201 {"id": "..."}
func (h *OrdemServicoHandler) Criar(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.Decodificar[dto.CriarOrdemServicoRequest](r) // JSON estrito + req.Validar()
	if err != nil {
		httpx.EscreverErro(w, r, err)
		return
	}
	sessao := httpx.SessaoDe(r.Context())
	id, err := cqrs.Send(r.Context(), h.d, req.ParaCommand(sessao.EmpresaID))
	if err != nil {
		httpx.EscreverErro(w, r, err)
		return
	}
	httpx.Escrever(w, http.StatusCreated, httpx.Criado{ID: id})
}

func (h *OrdemServicoHandler) Obter(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParamID(r, "id")
	if err != nil {
		httpx.EscreverErro(w, r, err)
		return
	}
	sessao := httpx.SessaoDe(r.Context())
	os, err := cqrs.Send(r.Context(), h.d, query.ObterOrdemServicoQuery{EmpresaID: sessao.EmpresaID, ID: id})
	if err != nil {
		httpx.EscreverErro(w, r, err)
		return
	}
	httpx.Escrever(w, http.StatusOK, os)
}
```

Por que esse formato é fixo: três passos, três `if err`. Se um handler tem um
quarto passo — um `if` sobre um campo, uma conta, uma segunda chamada ao
dispatcher para "completar" dados — a lógica pertence à aplicação
(`/criar-caso-de-uso`). O handler nunca lê `r.Body` direto, nunca escolhe um
status de erro, nunca monta entidade.

**A validação acontece antes de o handler ver os campos.**
`httpx.Decodificar` rejeita JSON malformado, campos desconhecidos
(`DisallowUnknownFields`) e corpo acima de 1 MB, e chama `Validar()` do DTO.
Se falhar, o handler devolve o erro e pronto — os campos nunca foram usados.

## DTO de entrada

```go
package dto

type CriarOrdemServicoRequest struct {
	ClienteID  string          `json:"cliente_id"`
	Itens      []ItemRequest   `json:"itens"`
	Observacao string          `json:"observacao"`
}

// Validar: só forma. "cliente existe?" é do caso de uso; "desconto ≤ subtotal" é do domínio.
func (r CriarOrdemServicoRequest) Validar() error {
	errs := []error{
		validar.Obrigatorio("cliente_id", r.ClienteID),
		validar.UUID("cliente_id", r.ClienteID),
		validar.TamanhoMax("observacao", r.Observacao, 2000),
		validar.TamanhoMaxLista("itens", len(r.Itens), 50),
	}
	for i, it := range r.Itens {
		errs = append(errs, validar.Prefixo(fmt.Sprintf("itens[%d]", i), it.Validar()))
	}
	return validar.Juntar(errs...)
}

// ParaCommand converte para o command. A empresa vem da sessão, nunca do corpo:
// o cliente da API não escolhe em que empresa está escrevendo.
func (r CriarOrdemServicoRequest) ParaCommand(empresaID shared.ID) command.CriarOrdemServicoCommand {
	clienteID, _ := shared.ParseID(r.ClienteID) // já validado
	…
}
```

Nomes de campo JSON em `snake_case` pt-BR, iguais aos do domínio quando
possível: o frontend usa os mesmos nomes nas mensagens de erro por campo.

## Respostas e erros

- `201` com `{"id"}` ao criar; `200` com o DTO da query ao ler; `204` sem corpo
  em ações sem retorno; `200` com `{"id","versao"}` em atualizações (o front
  precisa da nova versão para o próximo save).
- Erro **sempre** por `httpx.EscreverErro`, que mapeia `erros.Kind` → status e
  escreve `{"codigo","mensagem","campos"}`. `Interno` vira 500 com mensagem
  genérica e o erro real só no log — nunca vazar `pgx`/stack para o cliente.
- Listagens: `?pagina=1&por_pagina=20&status=...&busca=...` lidos com
  `httpx.Query(r).Int("pagina", 1)`; resposta `{"itens":[…],"total":N,"pagina":1,"por_pagina":20}`.

## Middlewares e rotas

```go
r := chi.NewRouter()
r.Use(middleware.RequestID, middleware.Recover(log), middleware.Log(log), middleware.CORS(cfg.OrigensPermitidas))
r.Get("/health", saude)
r.Route("/api/v1", func(r chi.Router) {
	r.Post("/auth/login", auth.Login)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Autenticacao(sessoes))
		ordemServico.Rotas(r)
		…
	})
})
```

- Recursos em pt-BR, plural, kebab-case: `/ordens-servico`, `/clientes`,
  `/servicos`. Ações que não são CRUD como sub-recurso com POST:
  `/ordens-servico/{id}/status`, `/ordens-servico/{id}/pagamento`.
- Autenticação: header `Authorization: Bearer <token>`; o middleware busca a
  sessão (hash do token) e coloca `httpx.Sessao{UsuarioID, EmpresaID}` no
  `ctx`. Rotas públicas: só `/health` e `/auth/login`.
- CORS: origens vêm de config (`CORS_ORIGENS`); nunca `*` com credenciais.

## Testes

`httptest` com um dispatcher real e handlers **fake** registrados, para testar
só o que é do adapter: decode, validação, status, formato do erro, sessão
chegando ao command.

```go
type fakeCriar struct{ recebido command.CriarOrdemServicoCommand }
func (f *fakeCriar) Handle(_ context.Context, c command.CriarOrdemServicoCommand) (shared.ID, error) { f.recebido = c; return idFixo, nil }

func TestCriarOrdemServico_CampoDesconhecidoDa400(t *testing.T) {
	d := cqrs.NewDispatcher()
	cqrs.Register(d, &fakeCriar{})
	srv := httptest.NewServer(rotasDeTeste(d)) // com sessão fixa no ctx
	resp := post(t, srv, "/api/v1/ordens-servico", `{"cliente_id":"…","inventado":1}`)
	if resp.StatusCode != 400 { … }
}
```

Casos mínimos por endpoint: feliz; corpo inválido (400 com `campos`);
`NaoEncontrado` do handler vira 404; `Conflito` vira 409; sem token vira 401.

## Checklist

- [ ] Rota registrada em `Rotas` do handler do módulo; caminho pt-BR plural
- [ ] Handler nos três passos; nenhum `if` de regra; nenhum status de erro escolhido à mão
- [ ] DTO com `Validar()` só de forma; `ParaCommand` recebe a empresa da sessão
- [ ] `httpx.Decodificar` (nunca `json.NewDecoder(r.Body)` direto)
- [ ] Resposta de sucesso é DTO da query ou `httpx.Criado`; nunca entidade de domínio
- [ ] Rota protegida está dentro do grupo com `Autenticacao`
- [ ] Testes: feliz, 400 com campos, 404, 409, 401
- [ ] `go list -deps ./internal/adapters/http | grep -E "adapters/postgres|pgx"` vazio
- [ ] Commit: `feat(http): endpoints de ordem de serviço` (ver `/commits`)
