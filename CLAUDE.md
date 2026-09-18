# autoReboque — backend

Sistema para empresas de guincho/reboque (guincheiras): orçamentos e ordens de
serviço, clientes, catálogo de serviços, motoristas, dados da empresa, fotos do
veículo, impressão e envio por WhatsApp. Produto genérico e vendível (white-label
leve): cada empresa configura logo, dados e preços. Primeiro cliente: Auto Socorro
Trevo (BA).

Este repositório é **só a API** (Go + PostgreSQL). O frontend vive em
`../frontend` (repositório `autoReboque-web`) e consome a API por HTTP/JSON.

Leia também: `docs/README.md` (índice), `docs/decisoes.md` (decisões de
arquitetura numeradas) e `docs/planos/` (planos de implementação).

## Estado atual — leia antes de tocar em código

O código em `internal/` (`ordemservico`, `cliente`, `web`, …) e `cmd/server` é a
**V2 legada**: monólito com `html/template` + HTMX e repositório dentro do
domínio. Ele funciona e é a referência das regras de negócio, mas **está sendo
substituído** pela arquitetura descrita abaixo, seguindo
`docs/planos/2026-09-18-arquitetura-hexagonal.md`.

Regras nesta transição:

- Funcionalidade nova entra **só** na arquitetura nova (`internal/domain`,
  `internal/application`, `internal/adapters`, `pkg`). Não estender o legado.
- O legado só recebe correção de bug bloqueante. É apagado na última fase do plano.
- Ao migrar um módulo, ler o legado para **preservar as regras** (transições de
  status, numeração, snapshot de cliente, dinheiro em centavos etc.), não copiar a
  estrutura.
- O sistema **não está em produção**: o esquema do banco pode ser reescrito
  (migrações novas do zero), sem migração de dados.

## Escopo por fase

Implementamos **um módulo por vez**, com atenção total: fundação → autenticação →
**ordem de serviço** (foco atual) → cadastros → fotos/impressão → desligar o
legado. Não abrir frentes fora da fase corrente; se surgir algo novo, registrar
no backlog (GitHub Projects) e seguir.

## Stack

| Coisa | Escolha | Observação |
|---|---|---|
| Linguagem | Go 1.27 | biblioteca padrão primeiro |
| Banco | PostgreSQL 17 via `pgx/v5` | SQL escrito à mão; sem ORM, sem query builder |
| Roteamento | `github.com/go-chi/chi/v5` | única lib de HTTP permitida |
| IDs | `uuid` da **stdlib** (Go 1.27), `uuid.NewV7()` | ordenado no tempo; bom para índice |
| Senhas | `golang.org/x/crypto/bcrypt` | |
| Logs | `log/slog` | |
| Validação, CQRS, erros | **próprios**, em `pkg/` e `internal/application/cqrs` | nada de lib de mediator/validator |
| Infra local | Docker Compose: Postgres, pgAdmin (`:5050`), API (`:8080`) | |

Antes de adicionar qualquer dependência: perguntar. A regra é "stdlib resolve?".

## Arquitetura: hexagonal (ports & adapters) + CQRS

```
cmd/
  api/                  composição (wiring) e start do servidor HTTP          [fase 0]
  seed/                 dados fictícios de desenvolvimento
  server/, hashsenha/   LEGADO (some na última fase)
internal/
  domain/               NÚCLEO — regras de negócio. Só stdlib + pkg/.
    shared/             EntidadeBase (auditoria), ID, guard clauses, Dinheiro
    ordemservico/       agregado OS: entidade, VOs, status, erros.go, repository.go (port)
    usuario/            usuário, credenciais, sessão (autenticação)
    cliente/ …          demais agregados, um pacote por agregado
  application/          CASOS DE USO — orquestra domínio + ports. Sem HTTP, sem SQL.
    cqrs/               Command, Query, Handler, Dispatcher, decorators (log, validação, transação)
    port/               interfaces "driven" que não são repositório: TxManager, Clock, PasswordHasher…
    ordemservico/
      command/          um arquivo por comando: criar_ordem_servico.go (+ _test.go)
      query/            um arquivo por consulta: obter_ordem_servico.go, listar_ordens_servico.go
    auth/               login, logout, sessão atual
  adapters/             IMPLEMENTAÇÕES dos ports
    http/               driving: router.go (chi), middleware/, handler/, dto/
    postgres/           driven: repositórios, SQL, transação, migrations/
pkg/                    FERRAMENTAS transversais, sem regra de negócio, só stdlib:
  erros/                taxonomia de erros (Kind + código estável + campos)
  validar/              regras de validação de entrada, compostas e sem reflexão
  httpx/                decode/encode JSON, mapeamento erro → status HTTP
  config/  logger/      variáveis de ambiente, slog
docs/                   decisões, planos, referência
.claude/skills/         skills por camada (ver "Skills")
```

### Regra de dependência (inviolável)

```
cmd → adapters → application → domain → pkg → stdlib
```

- `domain` importa **só** `pkg/` e stdlib. Nunca `application`, `adapters`, pgx, chi.
- `application` importa `domain` e `pkg`. Nunca `adapters`, pgx, chi, `net/http`.
- `adapters/http` importa `application` (dispatcher, DTOs) e `pkg`. Nunca `adapters/postgres`.
- `adapters/postgres` importa `domain` (entidades, ports) e `pkg`. Nunca `adapters/http`.
- `pkg/` importa só stdlib. É genérico: se um pacote de `pkg/` precisa saber o que é
  uma OS, ele está no lugar errado.
- Só `cmd/api/main.go` conhece todos os pacotes e faz o wiring.

Ao criar um pacote, conferir `go list -deps ./internal/domain/... | grep -v std`
(um script de verificação entra na fase 0 do plano).

### Domínio

- Todo agregado embute `shared.EntidadeBase`: `ID` (uuid v7), `CriadoEm`,
  `AtualizadoEm`, `ExcluidoEm *time.Time` (exclusão lógica) e `Versao`
  (optimistic locking). Quem cria/altera (`CriadoPor`/`AtualizadoPor`) entra
  quando houver usuários individuais — está previsto, não implementado.
- Construtores validam (`NovaOrdemServico(...) (*OrdemServico, error)`); um
  agregado nunca existe em estado inválido. Métodos que mudam estado devolvem
  `error` e recebem `agora time.Time` quando precisam de tempo (o domínio não
  chama `time.Now()`).
- **Guard clauses** (`shared/guard.go`) no começo dos construtores e métodos:
  `guard.NaoVazio("cliente.nome", nome)`, `guard.Entre(...)`, combinadas com
  `guard.Juntar(...)`. Devolvem `erros.Validacao` com o campo.
- **Erros ficam em `erros.go`** de cada pacote de domínio, como sentinelas
  (`var ErrTransicaoInvalida = erros.Regra("os.transicao_invalida", "…")`).
  Regras em `<agregado>.go`, `status.go` etc. nunca definem erro inline.
- **Port de repositório** em `repository.go` do próprio domínio (quem consome
  define a interface). Métodos mínimos e nomeados pelo caso de uso:
  `Salvar`, `ObterPorID`, `ProximoNumero`. Nada de repositório genérico.
- Dinheiro em **centavos `int64`**, quantidades em **centésimos `int64`**
  (`shared.Centavos`). Nunca float.
- Sem dependência de `time.Now()`, aleatoriedade ou I/O dentro do domínio.
  Testes de domínio são puros e rápidos.

### Aplicação (CQRS)

- **Command** escreve, **Query** lê. Nunca um comando devolve a entidade inteira;
  devolve o ID ou nada. Consultas devolvem DTOs de leitura (structs simples),
  nunca entidades de domínio.
- Struct de comando embute `cqrs.Command[R]`; de consulta, `cqrs.Query[R]`,
  onde `R` é o tipo do resultado. O handler implementa
  `Handle(ctx, req) (R, error)`. Registro no `cmd/api/main.go` com
  `cqrs.Register(d, handler)`; uso com `res, err := cqrs.Send(ctx, d, req)` —
  tipado, sem casts em quem chama. Reflexão só dentro do dispatcher
  (`reflect.TypeOf` para achar o handler).
- Decorators do dispatcher, nesta ordem: recover → log → validação
  (`Validar() error` do request, se existir) → transação (**só commands**).
  Handlers não abrem transação; o `port.TxManager` já envolveu a chamada e os
  repositórios enxergam a transação pelo `context`.
- Um handler de comando faz: carregar agregado(s) → chamar método do domínio →
  salvar. Se está calculando regra de negócio dentro do handler, a regra pertence
  ao domínio.
- Queries podem ter SQL próprio otimizado (read model) sem passar pelo agregado —
  ficam em `adapters/postgres` atrás de uma interface definida no pacote `query`.

### Adapters HTTP

- `router.go` só liga rota → handler → middleware. Zero lógica.
- Handler é magro e sempre no mesmo formato: decodificar + validar
  (`httpx.Decodificar[T]`, que rejeita campos desconhecidos e chama `Validar()`
  do DTO) → `cqrs.Send` → `httpx.Escrever`/`httpx.EscreverErro`. Sem `if` de
  regra de negócio, sem SQL, sem montar entidade.
- DTOs de entrada em `dto/` com `Validar() error` usando `pkg/validar` (formato,
  tamanho, obrigatoriedade). Regra de negócio **não** é validada aqui.
- Erros: um único mapeamento em `httpx.EscreverErro` (`erros.Kind` → status;
  corpo `{"codigo","mensagem","campos"}`). Handler nunca escolhe status de erro.
- Middlewares: request ID, log, recover, CORS (frontend em outra origem),
  autenticação (Bearer opaco). Rotas em `/api/v1/...`, recursos em pt-BR no
  plural (`/ordens-servico/{id}`).

### Adapters Postgres

- Um arquivo de repositório por agregado (`ordemservico_repository.go`) e o SQL
  em consts no arquivo irmão (`ordemservico_sql.go`). Colunas sempre explícitas
  (nunca `SELECT *`); `RETURNING` para ids/versão.
- **Proibido N+1**: filhos de uma lista vêm num único `WHERE pai_id = ANY($1)`
  e são distribuídos em memória; listagens têm `LIMIT` e ordenação explícita.
- Optimistic locking: `UPDATE … WHERE id = $1 AND versao = $2`; 0 linhas →
  `erros.Conflito`. Exclusão lógica: `excluido_em IS NULL` em toda leitura.
- Transação vem do `context` (`postgres.TxManager`); o repositório usa
  `querier(ctx)` que devolve a tx se houver, senão o pool.
- Migrações SQL numeradas em `adapters/postgres/migrations/NNN_nome.sql`,
  aplicadas na subida; toda tabela de negócio tem `empresa_id`, `criado_em`,
  `atualizado_em`, `excluido_em`, `versao`.

## Convenções de código

- **Idioma**: identificadores em pt-BR sem acento (`OrdemServico`, `MudarStatus`,
  `ErrNumeroDuplicado`); termos técnicos consagrados ficam em inglês como sufixo
  ou nome de pacote (`Command`, `Query`, `Handler`, `Repository`, `Dispatcher`,
  `Middleware`, `Request`/`Response`, `cqrs`, `http`, `postgres`). Comentários,
  mensagens de erro, logs e docs em pt-BR.
- Arquivos `snake_case.go`; um agregado por pacote; teste ao lado
  (`_test.go`). Pacotes com nome curto e sem underscore.
- Funções curtas, retorno cedo, sem variáveis globais mutáveis, sem `init()`.
- Contexto sempre como primeiro parâmetro; erros sempre tratados ou envolvidos
  com `%w` e contexto (`fmt.Errorf("salvar OS %s: %w", id, err)`).
- Código claro para quem está aprendendo Go: preferir o óbvio ao esperto;
  comentar o **porquê** de decisões, não o quê.
- `gofmt` e `go vet` limpos antes de todo commit.

## Testes

- **Domínio e aplicação**: unitários puros, com fakes escritos à mão
  (`fakeRepository` em `_test.go`), sem banco. Testar regras (transições,
  cálculos, guards) e erros esperados (`errors.Is`).
- **Postgres**: integração com `TEST_DATABASE_URL` (banco descartável
  `autoreboque_test`); `go test -p 1 ./...` porque os pacotes compartilham o
  banco. Sem a variável, os testes de integração são pulados.
- **HTTP**: `httptest` + dispatcher com handlers fake; testa decode, validação,
  status e formato do erro — não regra de negócio.
- Nome de teste diz a regra: `TestOrdemServico_NaoConcluiSemIniciar`.

```sh
docker compose up -d db pgadmin           # banco + pgAdmin (http://localhost:5050)
go run ./cmd/server                       # legado, enquanto existir
go test ./...                             # unitários
$env:TEST_DATABASE_URL = 'postgres://autoreboque:autoreboque@localhost:5432/autoreboque_test?sslmode=disable'
go test -p 1 ./...                        # com integração
```

Docker Desktop não sobe sozinho no Windows; após `docker compose up -d`,
esperar alguns segundos antes de rodar testes de integração.

## Skills

Use a skill correspondente **antes** de criar código de uma camada; elas trazem
o esqueleto, os nomes e os testes esperados.

| Skill | Quando |
|---|---|
| `/criar-dominio` | novo agregado, value object, regra ou erro de domínio |
| `/criar-caso-de-uso` | novo command/query, handler, registro no dispatcher |
| `/criar-repositorio` | implementação Postgres de um port, SQL, migração |
| `/criar-endpoint` | nova rota HTTP, DTO, validação de entrada, middleware |
| `/criar-ferramenta` | utilitário transversal em `pkg/` |
| `/commits` | como separar e escrever commits (também antes de `git push`) |

## Fora do escopo agora

Integração com seguradoras, motoristas com login, comissões, financeiro,
multiempresa com cadastro self-service, notificações. O desenho não deve fechar
essas portas (`empresa_id` em tudo, `origem_atendimento` na OS), mas nada disso
é implementado sem pedido explícito.
