# Plano de implementação — arquitetura hexagonal + CQRS

Data: 2026-09-18 · Status: **em execução** · Decisões base: D-002 a D-010 em
[`../decisoes.md`](../decisoes.md).

## Como usar este plano

- **Uma fase por vez, na ordem.** Cada fase termina com `go build ./... && go
  vet ./... && go test ./...` limpos, critérios de aceite marcados e commits
  feitos (ver `/commits`). Só então a próxima começa.
- Dentro da fase, seguir a ordem das entregas: elas foram encadeadas para que
  cada passo compile em cima do anterior.
- Antes de codar cada entrega, abrir a skill da camada (`/criar-dominio`,
  `/criar-caso-de-uso`, `/criar-repositorio`, `/criar-endpoint`,
  `/criar-ferramenta`). Os contratos em `references/` das skills são a
  especificação; se a implementação precisar divergir, atualizar o
  `references/` **no mesmo commit**.
- Marcar as caixas `[x]` aqui conforme avança e registrar desvios na seção
  "Diário" no fim. O plano é vivo; o que não está nele não é feito sem
  registrar.
- O legado é consultado para **regras**, nunca copiado. Arquivos-guia:
  `internal/ordemservico/{os,status,numero,item,dinheiro}.go`,
  `internal/database/migrations/001_schema.sql`, `internal/auth/*.go`.

## Visão final da árvore

```
backend/
  cmd/api/main.go
  cmd/seed/main.go
  internal/
    domain/
      shared/      id.go entidade.go dinheiro.go (+tests)
      guard/       guard.go (+tests)
      empresa/     empresa.go erros.go repository.go
      usuario/     usuario.go sessao.go erros.go repository.go
      cliente/     cliente.go erros.go repository.go
      servico/     servico.go erros.go repository.go
      motorista/   motorista.go erros.go repository.go
      ordemservico/ ordemservico.go item.go status.go numero.go veiculo.go trajeto.go pagamento.go erros.go repository.go
      imagem/      imagem.go erros.go repository.go storage.go (port)
    application/
      cqrs/        cqrs.go decorators.go (+tests)
      port/        clock.go tx.go senha.go token.go
      auth/        login.go logout.go sessao_atual.go autenticador.go
      empresa/     command/ query/
      cliente/     command/ query/
      servico/     command/ query/
      motorista/   command/ query/
      ordemservico/ command/ query/
      imagem/      command/ query/
    adapters/
      http/        router.go middleware/ handler/ dto/
      postgres/    postgres.go tx.go erros.go migrator.go testdb.go migrations/ *_repository.go *_sql.go *_leitor.go
      seguranca/   bcrypt.go token.go
      storage/     local.go (imagens em disco)
  pkg/ erros/ validar/ httpx/ config/ logger/ relogio/
  scripts/ verificar-deps.sh
  docs/ decisoes.md planos/ README.md
```

---

## Fase 0 — Fundação

**Objetivo.** Tudo que as demais fases usam, com testes, sem nenhuma regra de
negócio ainda. Ao fim, `cmd/api` sobe, responde `GET /health` e o script de
dependências passa.

### Entregas, na ordem

1. [ ] **`pkg/erros`** — `Erro`, `Kind`, construtores, `Is` por código,
   `Comf`, `ComCausa`, `KindDe`, `Juntar`, `Campo`, `Campos`. Testes:
   `errors.Is` após `Comf`; `Juntar` funde campos; `KindDe` de erro envolvido
   com `%w`. Contrato: `.claude/skills/criar-dominio/references/shared.md`.
2. [ ] **`pkg/validar`** — regras da tabela em
   `.claude/skills/criar-endpoint/references/httpx.md` (Obrigatorio,
   TamanhoMax/Entre, TamanhoMaxLista, UUID, Email, Telefone, Placa, Data,
   DataHora, UmDe, Inteiro, Que, Prefixo, Juntar). Teste de tabela por regra.
3. [ ] **`pkg/relogio`** — `Clock` interface, `Real{}`, `Fixo(t)`.
4. [ ] **`pkg/logger`** — `Novo(dev bool) *slog.Logger` (texto em dev, JSON
   fora); helper `Com(ctx)` que injeta `request_id` se houver.
5. [ ] **`pkg/config`** — `Carregar()` lendo `.env` + ambiente (portar
   `internal/config/dotenv.go`; não mover, o legado ainda usa o dele).
   Campos: `DATABASE_URL`, `HTTP_ADDR`, `CORS_ORIGENS` (lista por vírgula),
   `SESSAO_DURACAO`, `OS_NUMERO_INICIAL`, `STORAGE_DIR`, `UPLOAD_MAX_MB`,
   `DEV`. Erro claro por variável obrigatória ausente.
6. [ ] **`internal/domain/shared`** — `ID` (`uuid.UUID` da stdlib, `NovoID`
   v7, `ParseID`, `IDVazio`), `EntidadeBase` (+`NovaEntidadeBase`, `Tocar`,
   `Excluir`, `Excluida`, sentinelas `ErrJaExcluida`,
   `ErrVersaoDesatualizada`), `Centavos`/`Centesimos` (portar
   `internal/ordemservico/dinheiro.go` e testes).
7. [ ] **`internal/domain/guard`** — funções do contrato + `Juntar`. Testes.
8. [ ] **`internal/application/port`** — `Clock`, `TxManager`,
   `PasswordHasher`, `GeradorToken` (`Gerar() (token, hash string, err
   error)`, `Hash(token) string`).
9. [ ] **`internal/application/cqrs`** — exatamente
   `.claude/skills/criar-caso-de-uso/references/cqrs.md` (dispatcher +
   decorators Recover, Log, Validacao, Transacao). Testes listados lá.
10. [ ] **`pkg/httpx`** — `Decodificar[T]`, `ParamID`, `Query`, `Escrever`,
    `SemConteudo`, `EscreverErro` (tabela Kind→status), `Criado`,
    `Atualizado`, `Sessao`/`SessaoDe`/`ComSessao`. Testes com `httptest`:
    campo desconhecido → 400 com campo; cada Kind → status; Interno não vaza
    mensagem.
11. [ ] **`internal/adapters/postgres` base** — `postgres.go` (`Conectar(ctx,
    url) (*DB, error)`, `DB.Pool`, `QuerierDe`), `tx.go` (`Executar`),
    `erros.go` (`traduzErro`), `migrator.go` (portar
    `internal/database/migrator.go`: advisory lock + `schema_migrations`;
    embed de `migrations/`), `migrations/001_base.sql` (extensão `unaccent`;
    tabela `empresa` com colunas de auditoria — só `id`, `nome_fantasia`,
    `orcamento_validade_dias`, `os_numero_inicial` por enquanto; o resto entra
    na fase 3), `testdb.go` (`Conectar(t)`: skip sem `TEST_DATABASE_URL`,
    migra, `TRUNCATE` em cleanup). Teste: `Executar` faz rollback em erro;
    round-trip de um `uuid` gerado em Go (confirma o scan de `shared.ID`).
12. [ ] **`internal/adapters/http` base** — `router.go` (`NovoRouter(Deps)`),
    `middleware/` (`RequestID`, `Recover`, `Log`, `CORS`),
    `handler/saude.go` (`GET /health` → `{"status":"ok","banco":"ok"}` com
    ping no pool; 503 se o banco não responde). Teste do CORS (origem
    permitida × não permitida) e do recover (panic → 500 JSON).
13. [ ] **`cmd/api/main.go`** — config → logger → banco → migrações →
    dispatcher (sem handlers ainda) → router → `http.Server` com
    `ReadHeaderTimeout` e shutdown gracioso em `SIGINT/SIGTERM`.
14. [ ] **`scripts/verificar-deps.sh`** — falha se `go list -deps` de
    `internal/domain/...` contiver `application|adapters|pgx|chi`; de
    `internal/application/...` contiver `adapters|pgx|chi|net/http`; de
    `pkg/...` contiver `internal/` (exceção documentada: `httpx` → `chi`).
    Rodar no checklist de toda skill.
15. [ ] **Docker** — `Dockerfile` passa a compilar `./cmd/api`; compose
    mantém `api` em `:8080`. Legado, quando precisar rodar, no host com
    `HTTP_ADDR=:8090 go run ./cmd/server`.
16. [ ] **Docs** — CLAUDE.md: seção "Estado atual" ganha "fase 0 concluída em
    <data>"; README: comandos do `cmd/api`.

### Critérios de aceite

- [ ] `go test ./pkg/... ./internal/...` verde; com `TEST_DATABASE_URL`, os
  testes de `adapters/postgres` também.
- [ ] `docker compose up -d --build` → `curl localhost:8080/health` = 200.
- [ ] `scripts/verificar-deps.sh` passa.
- [ ] Nenhum pacote novo importa o legado (`internal/{web,ordemservico,…}`).

### Commits esperados

`feat(pkg): erros com taxonomia e códigos estáveis` · `feat(pkg): validar com
regras BR` · `feat(pkg): relogio, logger e config` · `feat(dominio/shared):
ID uuid v7, EntidadeBase e dinheiro` · `feat(dominio/guard): guard clauses` ·
`feat(aplicacao/cqrs): dispatcher tipado com decorators` · `feat(aplicacao):
ports Clock, TxManager, PasswordHasher, GeradorToken` · `feat(pkg/httpx):
decode estrito, respostas e mapeamento de erros` · `feat(postgres): pool,
transação no contexto, migrator e migração base` · `feat(http): router,
middlewares e health` · `feat(api): composição e subida do servidor` ·
`chore: script de verificação de dependências entre camadas` ·
`chore(docker): imagem compila cmd/api` · `docs: fase 0 concluída`.

---

## Fase 1 — Autenticação

**Objetivo.** Login com e-mail e senha por usuário de uma empresa; token
opaco; middleware que protege o resto da API (D-007).

### Domínio (`/criar-dominio`)

1. [ ] `domain/empresa` — `Empresa{EntidadeBase, NomeFantasia,
   OrcamentoValidadeDias, OSNumeroInicial}`, `NovaEmpresa`, `Repository{
   ObterPorID, Salvar}`. Mínimo para a OS; campos de documento na fase 3.
2. [ ] `domain/usuario` — `Usuario{EntidadeBase, EmpresaID, Nome, Email,
   SenhaHash, Ativo}`; `NovoUsuario(empresaID, nome, email, senhaHash,
   agora)` valida e-mail e normaliza (minúsculo, trim); `Desativar`,
   `TrocarSenha(hash, agora)`. `Sessao{EntidadeBase, UsuarioID, EmpresaID,
   TokenHash, ExpiraEm}`; `NovaSessao(...)`, `Expirada(agora)`. Erros:
   `ErrCredenciaisInvalidas` (NaoAutorizado, mensagem única para e-mail e
   senha errados), `ErrUsuarioInativo`, `ErrSessaoInvalida`, `ErrEmailEmUso`.
   Ports: `UsuarioRepository{ObterPorEmail, ObterPorID, Salvar}`,
   `SessaoRepository{Salvar, ObterPorTokenHash, Excluir, ExcluirExpiradas}`.

### Aplicação (`/criar-caso-de-uso`)

3. [ ] `application/auth/login.go` — `LoginCommand{Email, Senha}` →
   `LoginResultado{Token, ExpiraEm, Usuario{ID, Nome, Email}, Empresa{ID,
   NomeFantasia}}`. Handler: obter por e-mail → conferir (`PasswordHasher`) →
   usuário ativo → `GeradorToken.Gerar` → `NovaSessao` → salvar. Sempre o
   mesmo erro para e-mail inexistente e senha errada; sempre conferir o hash
   mesmo sem usuário (tempo constante).
4. [ ] `logout.go` — `LogoutCommand{TokenHash}` → apaga a sessão.
5. [ ] `sessao_atual.go` — `SessaoAtualQuery{}` lê `httpx.Sessao` do ctx e
   devolve usuário + empresa (para o front na abertura).
6. [ ] `autenticador.go` — `Autenticador{Validar(ctx, token) (Sessao,
   error)}` usado pelo middleware (chamada direta, sem dispatcher: roda em
   todo request e não deve poluir o log de casos de uso). Renova `ExpiraEm`
   se passou metade da validade (sessão deslizante).

### Adapters

7. [ ] `adapters/seguranca/bcrypt.go` (custo 12) e `token.go` (32 bytes
   `crypto/rand`, base64url; hash SHA-256 hex).
8. [ ] `adapters/postgres/migrations/002_usuario.sql` — `usuario` (unique
   `(empresa_id, email)` parcial em `excluido_em IS NULL`), `sessao`
   (`token_hash` PK, índice em `expira_em`).
9. [ ] `usuario_repository.go`, `sessao_repository.go` (+ `_sql.go`, testes de
   integração: e-mail duplicado → `ErrEmailEmUso`; sessão expirada não
   retorna).
10. [ ] `adapters/http/middleware/autenticacao.go` — lê Bearer, chama
    `Autenticador`, 401 com `auth.nao_autorizado`; coloca `httpx.Sessao`.
11. [ ] `handler/auth.go` — `POST /api/v1/auth/login` (200 com resultado),
    `POST /api/v1/auth/logout` (204), `GET /api/v1/auth/eu` (200). DTO
    `LoginRequest{Email, Senha}` com `Validar`.
12. [ ] `cmd/seed` — novo seed: cria a empresa "Auto Socorro Trevo" e o
    usuário admin (`SEED_ADMIN_EMAIL`, `SEED_ADMIN_SENHA` no `.env`, com
    padrão de desenvolvimento). Idempotente (não duplica se já existe).
13. [ ] Job de limpeza: `ExcluirExpiradas` a cada hora numa goroutine em
    `cmd/api` (simples; sem scheduler).

### Aceite

- [ ] Login errado → 401 com o mesmo corpo para e-mail e senha errados.
- [ ] Rota protegida sem token → 401; com token → 200 e `SessaoDe` preenchida.
- [ ] Logout invalida o token imediatamente.
- [ ] Testes de domínio, aplicação (fakes), postgres e http verdes.

**Commits:** `feat(dominio/empresa): agregado mínimo` · `feat(dominio/usuario):
usuário e sessão` · `feat(aplicacao/auth): login, logout e sessão atual` ·
`feat(seguranca): bcrypt e gerador de token` · `feat(postgres): migração 002
e repositórios de usuário e sessão` · `feat(http): autenticação com Bearer e
endpoints de auth` · `feat(seed): empresa e usuário admin` · `docs: fase 1
concluída`.

---

## Fase 2 — Ordem de serviço (foco)

**Objetivo.** Criar, consultar, editar, mudar status e registrar pagamento de
OS pela API, com numeração segura e locking otimista. Inclui o **mínimo** de
cliente, serviço e empresa para a OS existir (D-010).

### Domínio (`/criar-dominio`)

1. [ ] `domain/cliente` — `Cliente{EntidadeBase, EmpresaID, Tipo(PF/PJ), Nome,
   Documento, Telefone, Email, Categoria, Sistema, Ativo}`; `NovoCliente`;
   `Snapshot() ordemservico.ClienteSnapshot`… (atenção: para evitar ciclo, o
   snapshot é struct em `ordemservico` e a conversão fica no handler de
   aplicação, não no domínio de cliente). `Desativar`, `Atualizar`. Erros:
   `ErrNaoEncontrado`, `ErrDocumentoEmUso`, `ErrClienteDeSistema`. Port:
   `Repository{Salvar, ObterPorID, ExisteDocumento}`.
2. [ ] `domain/servico` — `Servico{EntidadeBase, EmpresaID, Nome, Unidade,
   PrecoCentavos, QuantidadeDoKm, Ativo, Ordem}`; só leitura nesta fase
   (`Repository{ObterPorID}`); CRUD na fase 3.
3. [ ] `domain/ordemservico` — portar o legado com `EntidadeBase` e `shared.ID`:
   - `ordemservico.go`: `OrdemServico` com `EmpresaID`, `Numero`, `Status`,
     `OrigemAtendimento`, datas (`DataEmissao`, `ValidoAte`, `AgendadaPara`,
     `AcionadoEm`, `AprovadaEm`, `ConcluidaEm`), `Cliente ClienteSnapshot`
     (ID + nome/documento/telefone), `Solicitante`, `Veiculo`, `Trajeto`,
     `Motorista MotoristaSnapshot`, `Guincho`, `RecebidoPor`, `Observacoes`,
     `Itens`, `DescontoCentavos`, `Pagamento`. Métodos: `NovaOrdemServico`,
     `AtualizarDados(...)`, `SubstituirItens([]Item)`, `AplicarDesconto`,
     `MudarStatus(novo, agendadaPara, validadeDias, agora)`,
     `RegistrarPagamento`, `Subtotal`, `Total`, `Saldo`, `StatusExibido`,
     `EhOrcamento`, `Acoes(agora) []Status`.
   - `status.go`: tabela de transições **idêntica ao legado**; `Rotulo`,
     `RotuloAcao`, `Icone`, `Exemplos` **não** vêm (são do frontend).
   - `item.go`, `veiculo.go` (`CategoriaVeiculo`), `trajeto.go`,
     `pagamento.go` (`StatusPagamento`, `FormaPagamento`; `Vencimento` só com
     `Faturado`), `numero.go` (`Numero{Ano, Sequencial}`, `String()` →
     `0142/2026`, `Parse`).
   - `erros.go`: `ErrNaoEncontrada`, `ErrTransicaoInvalida`,
     `ErrStatusInvalido`, `ErrItensBloqueados`, `ErrPagamentoInvalido`,
     `ErrAgendamentoObrigatorio` (AGENDADA exige `agendadaPara`).
   - `repository.go`: `Salvar`, `ObterPorID`, `ProximoNumero`.
   - Testes: **todas** as transições da tabela (permitidas e proibidas, via
     tabela de casos), expiração, renovação ao reabrir, carimbos de tempo,
     totais/desconto/saldo, pagamento faturado exige vencimento, itens
     bloqueados em concluída/cancelada.

### Aplicação (`/criar-caso-de-uso`)

4. [ ] `application/ordemservico/command/`:
   - `criar_ordem_servico.go` — `CriarOrdemServicoCommand{EmpresaID,
     ClienteID, Solicitante, Veiculo, Trajeto, MotoristaNome, Guincho,
     Itens []ItemInput, DescontoCentavos, Observacoes, DataEmissao?,
     AcionadoEm?}` → `shared.ID`. Cliente vazio → usa o cliente de sistema
     ("Serviço particular") da empresa. Número reservado na transação.
   - `atualizar_ordem_servico.go` — mesmos dados + `Versao`; carrega,
     `AtualizarDados`, `SubstituirItens`, `AplicarDesconto`, salva; conflito
     de versão vem do repositório.
   - `mudar_status_ordem_servico.go` — `{ID, Versao, Status, AgendadaPara?}`.
   - `registrar_pagamento.go` — `{ID, Versao, Status, ValorCentavos, Forma,
     Vencimento?}`.
   - `application/cliente/command/criar_cliente.go` (cadastro rápido de
     dentro da OS).
5. [ ] `application/ordemservico/query/`:
   - `obter_ordem_servico.go` — `OrdemServicoDetalhe` (tudo que a tela de
     detalhe e a impressão precisam, inclusive `acoes []string` calculadas
     pelo domínio e `status_exibido`).
   - `listar_ordens_servico.go` — filtros `Status` (inclui `ORCAMENTO_EXPIRADO`
     derivado: `status IN (ABERTO, ENVIADO) AND valido_ate < hoje`), `Busca`
     (número, cliente, placa; `unaccent`), `Fase` (orçamento/OS),
     paginação; linha com número, status exibido, cliente, veículo, total,
     pagamento, data.
   - `application/cliente/query/{listar_clientes,obter_cliente}.go` (busca
     por nome/documento/telefone, limite 20 para autocomplete).
   - `application/servico/query/listar_servicos.go` (ativos, por `ordem`).
   - `application/empresa/query/obter_empresa.go`.

### Adapters

6. [ ] `migrations/003_ordem_servico.sql` — `cliente`, `servico`,
   `sequencia_documento`, `ordem_servico`, `ordem_servico_item`. Colunas
   como no legado (`001_schema.sql`) + auditoria + `uuid`. Índices parciais
   das listagens. **Sem** `INSERT`s de seed na migração (cliente de sistema
   e catálogo são criados pelo seed/por empresa).
7. [ ] `cliente_repository.go`, `servico_repository.go` (só `ObterPorID`),
   `ordemservico_repository.go` + `_sql.go` (Salvar com itens por
   substituição; `ObterPorID` com itens; `ProximoNumero` por UPSERT),
   `ordemservico_leitor.go` (detalhe e listagem), `cliente_leitor.go`,
   `servico_leitor.go`, `empresa_leitor.go`.
   Testes: round-trip completo da OS; versão desatualizada; 20 goroutines em
   `ProximoNumero` sem repetição; listagem por status derivado; busca sem
   acento; OS de outra empresa invisível.
8. [ ] HTTP (`/criar-endpoint`):
   - `POST /api/v1/ordens-servico` (201) · `GET /api/v1/ordens-servico` ·
     `GET /api/v1/ordens-servico/{id}` · `PUT /api/v1/ordens-servico/{id}`
     (200 `{id, versao}`) · `POST /api/v1/ordens-servico/{id}/status` ·
     `POST /api/v1/ordens-servico/{id}/pagamento`.
   - `POST /api/v1/clientes` (201) · `GET /api/v1/clientes?busca=` ·
     `GET /api/v1/clientes/{id}`.
   - `GET /api/v1/servicos` · `GET /api/v1/empresa`.
   - DTOs com `Validar` (placa, telefone, datas ISO, status em `UmDe`,
     itens ≤ 50, quantidade/valor ≥ 0).
9. [ ] `cmd/seed` — cliente de sistema, catálogo padrão (7 serviços do
   legado), 3 clientes, 70 OS distribuídas por status/mês (portar
   `cmd/seed` legado usando os **commands** via dispatcher, não SQL direto —
   assim o seed também testa a API).

### Aceite

- [ ] Fluxo completo por `curl`/REST client: login → criar OS (número
  `0001/2026`) → adicionar itens via PUT → aprovar e agendar → iniciar →
  concluir → pagar. Cada resposta com status e corpo do contrato.
- [ ] Editar com `versao` antiga → 409 `entidade.versao_desatualizada`.
- [ ] Transição proibida → 422 `os.transicao_invalida`.
- [ ] Dois `POST` simultâneos não repetem número (teste de integração).
- [ ] Listagem com `status=ORCAMENTO_EXPIRADO` mostra só vencidos.
- [ ] Nenhuma query N+1 na listagem (verificado com log do pgx em dev).

**Commits:** `feat(dominio/cliente): agregado mínimo` · `feat(dominio/servico):
leitura do catálogo` · `feat(dominio/os): agregado OrdemServico com status,
itens e pagamento` · `feat(aplicacao/os): commands de criação, edição,
status e pagamento` · `feat(aplicacao/os): queries de detalhe e listagem` ·
`feat(aplicacao/cliente): cadastro rápido e busca` · `feat(postgres):
migração 003 (cliente, serviço, OS)` · `feat(postgres): repositórios e
leitores da OS` · `feat(http): endpoints de OS, clientes, serviços e
empresa` · `feat(seed): dados de desenvolvimento via commands` · `docs:
fase 2 concluída`.

---

## Fase 3 — Cadastros completos

**Objetivo.** CRUD de clientes, serviços, motoristas e configuração da
empresa; snapshots de motorista na OS.

1. [ ] `domain/motorista` (+ `MotoristaSnapshot` na OS ganha `ID`).
2. [ ] `domain/empresa` completa: razão social, CNPJ, endereço, cidade,
   telefone, WhatsApp, e-mail, `CorPrimaria` (só documentos),
   `OrcamentoCondicoes`, `LogoChave`.
3. [ ] Commands/queries: cliente (atualizar, desativar, listar paginado com
   filtros, detalhe com histórico de OS), serviço (criar, atualizar,
   reordenar, desativar), motorista (criar, atualizar, desativar),
   empresa (atualizar dados).
4. [ ] Migração `004_cadastros.sql` (motorista; colunas novas em `empresa`;
   `motorista_id` na OS).
5. [ ] Endpoints `/clientes` (PUT, DELETE lógico), `/servicos` (POST, PUT,
   DELETE), `/motoristas`, `/empresa` (PUT).
6. [ ] Aceite: catálogo editado não altera itens de OS antigas (snapshot);
   cliente com OS não pode ser excluído, só desativado.

---

## Fase 4 — Fotos, dashboard e documentos

1. [ ] `domain/imagem` + port `Storage` (portar `internal/storage` e
   `internal/imagem`: validação JPEG/PNG/WebP, miniatura 480px, EXIF).
   Migração `005_imagem.sql`. Endpoints: `POST
   /ordens-servico/{id}/imagens` (multipart, ≤ `UPLOAD_MAX_MB`), `GET
   /imagens/{chave}` (com sessão), `DELETE`. Logo da empresa pelo mesmo
   storage.
2. [ ] `application/ordemservico/query/dashboard.go` — a receber, faturado no
   mês, orçamentos aguardando, série de 6 meses, formas de pagamento (SQL
   agregado; regras do legado `dashboard.go`). `GET /api/v1/dashboard`.
3. [ ] Documento: `GET /ordens-servico/{id}/documento` devolve o JSON completo
   para impressão/PDF (cabeçalho da empresa + OS); o frontend renderiza e
   imprime. Texto do WhatsApp gerado no frontend a partir do mesmo JSON.

---

## Fase 5 — Desligar o legado

1. [ ] Frontend consumindo todas as rotas (pré-requisito, repositório
   `autoReboque-web`).
2. [ ] Apagar `internal/{web,ordemservico,cliente,servico,motorista,empresa,
   auth,storage,imagem,documento,database,config}`, `cmd/server`,
   `cmd/hashsenha`; `go mod tidy`.
3. [ ] CLAUDE.md sem a seção "Estado atual"; README final; compose só com
   `api`; `.env.example` sem `SENHA_HASH`.
4. [ ] Aceite: `go build ./...` sem legado; `scripts/verificar-deps.sh`;
   suíte completa verde.

---

## Riscos e pontos de atenção

- **`shared.ID` no pgx.** `uuid.UUID` da stdlib é `[16]byte`; o pgx deve
  escanear direto. Confirmar no teste da fase 0 (entrega 11). Se falhar,
  converter na fronteira (`string`), nunca trocar o tipo do domínio.
- **Datas.** O legado usa `date` para emissão/validade com meia-noite UTC.
  Manter: `DataEmissao`/`ValidoAte` são `time.Time` só-data em UTC;
  comparações de expiração usam `somenteData(agora)`.
- **Número inicial.** `OS_NUMERO_INICIAL` só vale na primeira sequência do
  ano; passa a ser campo da empresa (`os_numero_inicial`) na fase 1 para
  não depender de env.
- **Bash no Windows** trava com heredocs > 8 KB: arquivos grandes via
  ferramenta de escrita, não via `cat <<EOF`.
- **Testes de integração** compartilham o banco: sempre `-p 1`; esperar o
  Postgres após `docker compose up`.
- **Legado divergindo.** Nunca corrigir regra só no legado; se um bug de
  regra for achado lá, corrigir no domínio novo (e no legado só se estiver
  em uso).
- **Escopo.** Se durante uma fase aparecer algo de outra fase, anotar aqui
  no diário e seguir.

## Diário

- 2026-09-18 — plano criado; repositórios separados; skills e CLAUDE.md
  publicados. Próximo passo: fase 0, entrega 1.
