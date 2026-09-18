# Decisões de arquitetura

Registro numerado das decisões que moldam o backend. Cada uma tem contexto,
decisão e consequências. Para mudar uma decisão, adicionar uma nova que a
substitui (não editar a antiga) e apontar a substituição.

Formato: **D-NNN — título** · data · status (aceita / substituída por D-XXX).

---

## D-001 — Backend e frontend em repositórios separados · 2026-09-18 · aceita

**Contexto.** A V2 misturava API, templates HTML e um design system React no
mesmo repositório Go. O produto vai ter uma interface própria, com design
elaborado, e o backend precisa evoluir sem arrastar o front.

**Decisão.** Dois repositórios: `autoReboque` (esta API, Go) e
`autoReboque-web` (frontend). Localmente, `autoReboque/backend` e
`autoReboque/frontend`. A API é a única fonte de dados do frontend.

**Consequências.** A API precisa de CORS, autenticação por token e contratos
JSON estáveis (códigos de erro). Templates e HTMX do legado somem quando o
frontend cobrir as telas.

## D-002 — Arquitetura hexagonal com um pacote por agregado · 2026-09-18 · aceita

**Contexto.** O legado usa package-by-feature com repositório dentro do
domínio; testar regra exige banco e a camada web conhece SQL.

**Decisão.** `internal/domain` (regras, ports), `internal/application`
(casos de uso), `internal/adapters/{http,postgres}` (implementações), `pkg/`
(ferramentas). Regra de dependência
`cmd → adapters → application → domain → pkg → stdlib`. Ports de repositório
definidos no domínio; demais ports em `application/port`.

**Consequências.** Mais arquivos por funcionalidade; em troca, domínio
testável sem I/O e adapters substituíveis. Um script confere as dependências.

## D-003 — CQRS com dispatcher próprio e tipado · 2026-09-18 · aceita

**Contexto.** Queremos separar escrita de leitura (leituras otimizadas em
SQL, escritas auditáveis em transação) e um ponto único para log, validação
e transação — sem adotar uma biblioteca de mediator.

**Decisão.** Pacote `internal/application/cqrs`: `Command[R]`/`Query[R]`
embutidos nos requests, `Handler[Req, R]`, `Dispatcher` com decorators
(recover, log, validação, transação só em commands). Reflexão apenas em
`Register`/`Send` (`reflect.TypeOf`). Validado em Go 1.27 (inferência de `R`).

**Consequências.** Todo caso de uso é um tipo + handler; handlers HTTP viram
três passos. Queries devolvem DTOs e podem ter SQL próprio (`Leitor`).

## D-004 — IDs UUID v7 da stdlib; auditoria em `EntidadeBase` · 2026-09-18 · aceita

**Contexto.** IDs sequenciais vazam volume e complicam merge futuro; o Go 1.27
tem `uuid` na biblioteca padrão com `NewV7()`.

**Decisão.** `shared.ID = uuid.UUID`, gerado em Go com v7 (ordenado no tempo).
Toda entidade persistida embute `EntidadeBase{ID, CriadoEm, AtualizadoEm,
ExcluidoEm, Versao}`. Exclusão é sempre lógica. `CriadoPor/AtualizadoPor`
entram quando houver usuários individuais.

**Consequências.** Colunas `uuid` no Postgres; toda leitura filtra
`excluido_em IS NULL`; optimistic locking padrão via `versao`.

## D-005 — Erros com taxonomia única (`pkg/erros`) · 2026-09-18 · aceita

**Decisão.** Um tipo `Erro{Kind, Codigo, Mensagem, Campos}`; sentinelas por
pacote de domínio em `erros.go`; `errors.Is` compara por `Codigo`. O adapter
HTTP mapeia `Kind → status` em um único lugar; o corpo de erro é
`{codigo, mensagem, campos}`.

**Consequências.** Handlers nunca escolhem status; o frontend confia no
`codigo`. Erros de driver nunca vazam (traduzidos na fronteira Postgres).

## D-006 — SQL à mão com pgx; sem ORM; sem N+1 · 2026-09-18 · aceita

**Decisão.** Queries em consts, colunas explícitas, filhos por `ANY($1)`,
filhos salvos por substituição em lote, transação via `context`
(`QuerierDe(ctx)`), migrações SQL numeradas embutidas.

**Consequências.** Mais SQL escrito, porém legível e previsível; testes de
integração cobrem round-trip, versão e tenant.

## D-007 — Autenticação por token opaco em sessão no banco · 2026-09-18 · aceita

**Contexto.** Frontend separado precisa de token; JWT exigiria lib ou
implementação própria de assinatura e não é revogável.

**Decisão.** `POST /auth/login` (e-mail + senha, bcrypt) devolve token
aleatório (32 bytes, base64url); o banco guarda só o SHA-256 dele com
validade. `Authorization: Bearer <token>`. Logout apaga a sessão. Usuário
pertence a uma empresa (`empresa_id` na sessão).

**Consequências.** Uma consulta por request (indexada por hash); revogação
imediata; sem dependência nova.

## D-008 — Idioma dos identificadores · 2026-09-18 · aceita

**Decisão.** Domínio e nomes de negócio em pt-BR sem acento; padrões
técnicos consagrados em inglês como sufixo/pacote (`Command`, `Query`,
`Handler`, `Repository`, `Dispatcher`, `cqrs`, `http`, `postgres`).
Comentários, erros, logs, docs e commits em pt-BR.

## D-009 — Migração do legado por substituição, sem migração de dados · 2026-09-18 · aceita

**Contexto.** O sistema ainda não está em produção; os dados são seed.

**Decisão.** A arquitetura nova nasce em pacotes novos (`domain`,
`application`, `adapters`, `cmd/api`) com esquema novo (uuid). O legado
(`internal/{web,ordemservico,…}`, `cmd/server`) continua compilando como
referência de regras e é apagado na última fase do plano. Nenhuma
funcionalidade nova entra no legado.

**Consequências.** Durante a transição existem dois binários; o compose sobe
o legado até o `cmd/api` cobrir o mesmo. Se o sistema entrar em produção
antes do fim da migração, esta decisão precisa ser revista (script de
migração de dados).

## D-010 — Um módulo por vez, OS primeiro · 2026-09-18 · aceita

**Decisão.** Ordem: fundação → autenticação → ordem de serviço → cadastros →
fotos/impressão → desligar legado. Cada fase fecha com testes e commits
próprios antes da próxima começar.

**Consequências.** A OS precisa de cliente e empresa mínimos para existir;
eles entram na fase da OS só com o necessário (ver plano).
