---
name: criar-repositorio
description: Implementa persistência em internal/adapters/postgres no padrão do autoReboque — repositório Postgres para um port do domínio, leitor (read model) para queries, SQL escrito à mão com pgx sem N+1, transação via context, migrações SQL numeradas e testes de integração com banco descartável. Use sempre que o pedido envolver "repositório", "repository", "persistir", "salvar no banco", "SQL", "query otimizada", "migração/migration", "tabela", "índice", "leitor/read model" ou "N+1", mesmo que o usuário só diga "liga isso no Postgres".
---

# Criar repositório (adapter Postgres)

O adapter Postgres implementa os ports que o domínio (`Repository`) e a
aplicação (`Leitor`, `TxManager`) definem. Aqui mora **todo** o SQL do sistema;
fora daqui ninguém importa `pgx`. A referência de SQL e dos helpers está em
[references/sql.md](references/sql.md).

## Arquivos

```
internal/adapters/postgres/
  postgres.go                    Pool (pgxpool), Conectar(cfg), QuerierDe(ctx)
  tx.go                          TxManager (port.TxManager): abre tx e a põe no ctx
  erros.go                       traduzErro(err): pgx.ErrNoRows / unique_violation → erros.*
  migrator.go                    aplica migrations/ na subida (advisory lock, tabela schema_migrations)
  migrations/
    001_base.sql                 empresa, usuario, sessao
    002_ordem_servico.sql        …uma migração por agregado/fase, nunca editada depois de commitada
  testdb.go                      helper de teste: conecta em TEST_DATABASE_URL, migra, limpa
  <agregado>_repository.go       implementa <agregado>.Repository
  <agregado>_sql.go              consts com o SQL do repositório e do leitor
  <agregado>_leitor.go           implementa query.Leitor (read model), quando o módulo tem queries
  <agregado>_repository_test.go  integração (pulado sem TEST_DATABASE_URL)
```

SQL em `const` no arquivo `_sql.go` irmão: quem lê o repositório vê só o fluxo;
quem quer o SQL abre um arquivo e vê todas as queries do agregado juntas.

## Padrões (e o porquê)

**Transação vem do `context`.** O decorator `Transacao` do dispatcher chama
`TxManager.Executar`, que guarda a `pgx.Tx` no `ctx`. Todo método de
repositório começa com `q := QuerierDe(ctx)` (devolve a tx se houver, senão o
pool). Assim o mesmo repositório serve dentro e fora de transação e a
aplicação nunca vê `pgx`.

```go
func (r *OrdemServicoRepository) Salvar(ctx context.Context, os *ordemservico.OrdemServico) error {
	q := QuerierDe(ctx)
	if os.Versao == 1 && !os.jaPersistida { … } // ver references/sql.md: insert x update
```

**Colunas explícitas, sempre.** `SELECT *` quebra em silêncio quando a tabela
muda de ordem. Uma const `colunasOrdemServico` reutilizada nos `SELECT`s
mantém um lugar só para manter.

**Optimistic locking no `UPDATE`.**
`UPDATE … SET …, versao = versao + 1 WHERE id = $1 AND versao = $2`; zero
linhas afetadas → `shared.ErrVersaoDesatualizada`. O repositório atualiza
`os.Versao` com o `RETURNING versao`.

**Filhos em uma query só (proibido N+1).** Para carregar N ordens com seus
itens: uma query para as ordens, **uma** para todos os itens com
`WHERE ordem_servico_id = ANY($1)` e distribuição em memória por
`map[shared.ID][]Item`. Nunca uma query por linha dentro de um `for`.

**Salvar filhos por substituição.** Itens de OS são value objects da raiz: no
`Salvar`, `DELETE FROM ordem_servico_item WHERE ordem_servico_id = $1` e
reinserção em lote com `pgx.Batch` ou `CopyFrom`. Simples, correto e dentro da
mesma transação.

**Exclusão lógica e tenant em toda leitura.** Todo `SELECT` de agregado tem
`WHERE empresa_id = $1 AND … AND excluido_em IS NULL`. Esquecer o
`empresa_id` é vazamento entre empresas; esquecer o `excluido_em` ressuscita
registro apagado.

**Reconstituir sem passar pelo construtor.** O construtor do domínio valida a
criação; o que está no banco já foi validado. O repositório monta a struct
direto (campos exportados) a partir de uma struct `linhaOrdemServico`
escaneada — conversão em `paraDominio()`, uma só, testada.

**Erros traduzidos na fronteira.** `traduzErro` converte `pgx.ErrNoRows` no
`ErrNaoEncontrado` do agregado e `unique_violation` (23505) em
`erros.Conflito` com código por constraint. Acima daqui ninguém compara com
`pgx.ErrNoRows`.

**Leitor (read model) é livre.** Queries de tela (`Listar`, `ObterDetalhe`,
dashboard) fazem `JOIN`, `COUNT(*) OVER()` para o total, agregações — o que
for mais rápido — e escaneiam direto no DTO da aplicação. Não passam pela
entidade. Paginação por `LIMIT $n OFFSET $m` com `PorPagina` ≤ 100 e
ordenação estável (`ORDER BY ano DESC, numero DESC`).

**Numeração por `UPSERT` com lock de linha.** `ProximoNumero` faz
`INSERT … ON CONFLICT (empresa_id, tipo, ano) DO UPDATE SET ultimo_numero =
sequencia.ultimo_numero + 1 RETURNING ultimo_numero` dentro da transação do
command: o segundo usuário espera o lock e recebe o número seguinte. Sem
`SELECT MAX`.

## Migrações

- Arquivo `NNN_nome.sql` em `migrations/`, `//go:embed`, aplicado em ordem na
  subida com advisory lock (dois processos subindo ao mesmo tempo não
  duplicam). Tabela `schema_migrations(versao, aplicada_em)`.
- **Nunca editar migração já commitada**; criar a próxima.
- Toda tabela de negócio: `id uuid PRIMARY KEY`, `empresa_id uuid NOT NULL
  REFERENCES empresa(id)`, `versao integer NOT NULL DEFAULT 1`, `criado_em`,
  `atualizado_em timestamptz NOT NULL`, `excluido_em timestamptz NULL`.
  Índice parcial em `(empresa_id, …) WHERE excluido_em IS NULL` para as
  listagens.
- Texto opcional é `NOT NULL DEFAULT ''`; dinheiro `bigint` em centavos;
  enums `text` + `CHECK` (mais fácil de evoluir que `CREATE TYPE`).
- Comentar no SQL o **porquê** de índices e constraints não óbvios.

## Testes de integração

```go
func TestOrdemServicoRepository_SalvarEObter(t *testing.T) {
	db := testdb.Conectar(t) // t.Skip se TEST_DATABASE_URL vazio; migra; TRUNCATE no Cleanup
	repo := postgres.NovoOrdemServicoRepository(db)
	…
}
```

- Cada teste parte de banco limpo (`TRUNCATE … CASCADE` no `t.Cleanup`).
- Testar: salvar e recarregar igual (round-trip), versão desatualizada →
  `ErrVersaoDesatualizada`, filtro por empresa (OS de outra empresa não
  aparece), excluída não aparece, numeração concorrente (goroutines
  disputando `ProximoNumero` não repetem número), listagem não faz N+1
  (contar queries com um tracer do pgx é o caminho, quando valer a pena).
- Rodar com `go test -p 1 ./internal/adapters/postgres/...`.

## Checklist

- [ ] Nenhum `SELECT *`; `empresa_id` e `excluido_em IS NULL` em todo SELECT de agregado
- [ ] `UPDATE` com `versao` e tratamento de 0 linhas
- [ ] Filhos carregados com `ANY($1)`; nenhuma query dentro de loop
- [ ] `QuerierDe(ctx)` em todo método; nenhum `Begin` fora de `tx.go`
- [ ] Erros passam por `traduzErro`; `pgx.ErrNoRows` não vaza
- [ ] Migração nova numerada; colunas de auditoria presentes
- [ ] Leitor escaneia direto no DTO; paginação com limite
- [ ] Testes de integração para round-trip, versão, tenant e exclusão lógica
- [ ] `go list -deps ./internal/adapters/postgres | grep -E "adapters/http|chi"` vazio
- [ ] Commits: `feat(postgres): migração da OS` e `feat(postgres): repositório da OS` separados (ver `/commits`)
