---
name: commits
description: Regras de commit e push do autoReboque — Conventional Commits em pt-BR com escopo por camada (dominio, aplicacao, postgres, http, pkg, docker, docs, claude), um commit por unidade lógica, quando commitar durante uma tarefa, o que nunca entra num commit e como fazer push. Use sempre antes de `git commit`, `git push`, ao terminar uma fase/skill, quando o usuário pedir para "commitar", "subir", "salvar no git", "organizar os commits" ou "separar em commits".
---

# Commits

Commits pequenos e nomeados pelo que mudam são a documentação mais lida do
projeto: `git log` precisa contar a história da arquitetura sem abrir o diff.

## Formato

```
<tipo>(<escopo>): <resumo no imperativo, minúsculo, sem ponto, ≤ 72 chars>

<corpo opcional: o PORQUÊ, não o quê; quebras de linha em 72 colunas>

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
```

O rodapé `Co-Authored-By` vai em todo commit que o Claude fez ou ajudou a fazer.

**Tipos:** `feat` (funcionalidade), `fix` (bug), `refactor` (sem mudar
comportamento), `test` (só testes), `docs`, `chore` (build, deps, config),
`perf`, `ci`.

**Escopos** — o mesmo vocabulário das camadas, para o log mostrar onde cada
mudança caiu:

| Escopo | Área |
|---|---|
| `dominio` | `internal/domain/*` (pode especificar: `dominio/os`, `dominio/cliente`) |
| `aplicacao` | `internal/application/*` (`aplicacao/cqrs`, `aplicacao/os`) |
| `postgres` | `internal/adapters/postgres` (repositórios, migrações, leitores) |
| `http` | `internal/adapters/http` (rotas, handlers, DTOs, middlewares) |
| `pkg` | `pkg/*` (`pkg/validar`, `pkg/erros`) |
| `api` | `cmd/api` (wiring, config de subida) |
| `seed` | `cmd/seed` |
| `docker` | Dockerfile, compose, `docker/` |
| `docs` | `docs/`, README, CLAUDE.md |
| `claude` | `.claude/skills`, `.claude/settings.json` |
| `legado` | qualquer toque no código antigo (`internal/web`, `cmd/server`…) |

**Exemplos:**

```
feat(dominio/os): agregado OrdemServico com itens e transições de status
feat(aplicacao/os): command CriarOrdemServico com numeração na transação
feat(postgres): migração 002 e repositório de ordem de serviço
feat(http): endpoints de criação e consulta de OS
test(postgres): numeração concorrente não repete número
refactor(aplicacao/cqrs): decorator de transação ignora queries
fix(http): 409 em vez de 500 quando a versão está desatualizada
chore(docker): pgAdmin com servidor pré-cadastrado
docs: plano da fase 2 (ordem de serviço)
```

## Granularidade: um commit por unidade lógica

Uma "unidade lógica" é algo que compila, passa nos testes e faz sentido
sozinho no log. Na prática, ao seguir as skills, a sequência natural de uma
funcionalidade é:

1. `feat(dominio/x): …` — agregado + testes
2. `feat(aplicacao/x): …` — commands/queries + testes
3. `feat(postgres): migração …` e `feat(postgres): repositório …` (separados:
   a migração é revisada com outro olhar)
4. `feat(http): …` — endpoints + testes
5. `chore(api): registra handlers de x` (se não coube no commit 4)
6. `docs:` — marcar a fase no plano, atualizar CLAUDE.md se um contrato mudou

Regras:

- **Nunca misturar camadas** num commit, exceto quando o wiring em `cmd/api`
  é uma linha e faz parte de "ligar o endpoint".
- **Nunca misturar `refactor` com `feat`.** Se precisou reorganizar antes de
  adicionar, são dois commits: primeiro o refactor (comportamento igual,
  testes passando), depois a funcionalidade.
- **Testes vão junto do código que testam**, no mesmo commit. `test(...)`
  sozinho é para testes adicionados depois.
- **Formatação/rename em massa** (gofmt, renomear pacote) é um commit próprio
  `chore`/`refactor`, para o diff da funcionalidade ficar legível.
- Um commit não deixa o `main` quebrado: `go build ./... && go vet ./... &&
  go test ./...` antes de cada um.

## Quando commitar durante uma tarefa

Commitar **ao fechar cada camada** de uma funcionalidade, não só no fim da
tarefa. Se a sessão cair, o trabalho até ali está salvo e revisável. Antes
de commitar, `git status` e `git diff --stat` para conferir que só o
pretendido está no stage — usar `git add <arquivos>` explícito, nunca
`git add -A` às cegas.

## O que nunca entra

- `.env`, senhas, tokens, hashes reais (o `.env.example` tem um hash de
  exemplo e é o único permitido).
- `bin/`, `dados/`, saídas de build, `node_modules`.
- Código comentado "para depois", `TODO` sem issue, `fmt.Println` de debug.
- Arquivos de outra funcionalidade "que estavam por perto".

## Push

- `git push` só depois de build, vet e testes limpos localmente.
- Trabalho direto na `main` está ok neste estágio (uma pessoa). Quando houver
  mais gente, feature branches + PR.
- Nunca `--force` na `main`. Se precisou desfazer algo já publicado, `git
  revert`.
- Antes do push, ler `git log origin/main..HEAD --oneline` e conferir que
  cada linha conta uma história clara. Se dois commits são "parte 1/parte
  2" da mesma coisa, `git rebase -i` local para juntar (só commits ainda não
  publicados).
