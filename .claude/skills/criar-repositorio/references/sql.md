# Referência de SQL e helpers do adapter Postgres

## Helpers base (`postgres.go`, `tx.go`, `erros.go`)

```go
package postgres

// Querier é o que pool e tx têm em comum. Repositórios só usam isto.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

type chaveTx struct{}

// QuerierDe devolve a transação do ctx, se o decorator Transacao abriu uma,
// senão o pool. É o que permite o mesmo repositório servir commands e queries.
func (db *DB) QuerierDe(ctx context.Context) Querier {
	if tx, ok := ctx.Value(chaveTx{}).(pgx.Tx); ok {
		return tx
	}
	return db.pool
}

// TxManager implementa port.TxManager.
func (db *DB) Executar(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(chaveTx{}).(pgx.Tx); ok {
		return fn(ctx) // já dentro de transação: reaproveita
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return erros.Interno(fmt.Errorf("abrir transação: %w", err))
	}
	defer tx.Rollback(ctx) // no-op depois do Commit
	if err := fn(context.WithValue(ctx, chaveTx{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// traduzErro converte erros do driver em erros do sistema. naoEncontrado é o
// sentinela do agregado; conflitos são mapeados por nome de constraint.
func traduzErro(err error, naoEncontrado *erros.Erro, conflitos map[string]*erros.Erro) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return naoEncontrado
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if e, ok := conflitos[pgErr.ConstraintName]; ok {
			return e
		}
		return erros.Conflito("banco.duplicado", "registro duplicado").ComCausa(err)
	}
	return erros.Interno(err)
}
```

## Repositório: inserir x atualizar

O agregado não sabe se já foi persistido. Duas opções; usamos a primeira:

1. `Salvar` tenta `UPDATE … WHERE id = $1 AND versao = $2`; se afetou 0 linhas,
   verifica `EXISTS(id)`: existe → `ErrVersaoDesatualizada`; não existe →
   `INSERT`. Uma ida a mais só no caso raro.
2. `INSERT … ON CONFLICT (id) DO UPDATE … WHERE ordem_servico.versao = $n` —
   uma query, mas o SQL fica longo e o erro de versão exige checar `RETURNING`.

```go
const sqlAtualizarOrdemServico = `
UPDATE ordem_servico SET
    status = $3, data_emissao = $4, valido_ate = $5, aprovada_em = $6, concluida_em = $7,
    cliente_id = $8, cliente_nome = $9, cliente_documento = $10, cliente_telefone = $11,
    desconto_centavos = $12, subtotal_centavos = $13, total_centavos = $14,
    atualizado_em = $15, versao = versao + 1
WHERE id = $1 AND versao = $2 AND excluido_em IS NULL
RETURNING versao`

const sqlInserirOrdemServico = `
INSERT INTO ordem_servico (
    id, empresa_id, numero, ano, versao, status, data_emissao, valido_ate,
    cliente_id, cliente_nome, cliente_documento, cliente_telefone,
    desconto_centavos, subtotal_centavos, total_centavos, criado_em, atualizado_em
) VALUES ($1, $2, $3, $4, 1, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)`
```

`subtotal_centavos`/`total_centavos` são gravados (desnormalizados) para as
listagens e o dashboard não somarem itens toda vez; o valor vem de
`os.Subtotal()`/`os.Total()` na hora de salvar.

## Filhos: substituir em lote

```go
const sqlApagarItens = `DELETE FROM ordem_servico_item WHERE ordem_servico_id = $1`

func (r *OrdemServicoRepository) salvarItens(ctx context.Context, q Querier, os *ordemservico.OrdemServico) error {
	if _, err := q.Exec(ctx, sqlApagarItens, os.ID); err != nil {
		return erros.Interno(err)
	}
	if len(os.Itens) == 0 {
		return nil
	}
	linhas := make([][]any, 0, len(os.Itens))
	for _, it := range os.Itens {
		linhas = append(linhas, []any{os.ID, it.Posicao, it.ServicoID, it.Descricao, string(it.Unidade), int64(it.QuantidadeCentesimos), int64(it.ValorUnitario), int64(it.Subtotal())})
	}
	_, err := q.CopyFrom(ctx, pgx.Identifier{"ordem_servico_item"},
		[]string{"ordem_servico_id", "posicao", "servico_id", "descricao", "unidade", "quantidade_centesimos", "valor_unitario_centavos", "subtotal_centavos"},
		pgx.CopyFromRows(linhas))
	return erros.Interno(err)
}
```

## Carregar N pais com filhos (sem N+1)

```go
const sqlItensDe = `
SELECT ordem_servico_id, posicao, servico_id, descricao, unidade, quantidade_centesimos, valor_unitario_centavos
FROM ordem_servico_item
WHERE ordem_servico_id = ANY($1)
ORDER BY ordem_servico_id, posicao`

func (r *OrdemServicoRepository) carregarItens(ctx context.Context, q Querier, ids []shared.ID) (map[shared.ID][]ordemservico.Item, error) {
	rows, err := q.Query(ctx, sqlItensDe, ids)
	if err != nil {
		return nil, erros.Interno(err)
	}
	defer rows.Close()
	porOS := map[shared.ID][]ordemservico.Item{}
	for rows.Next() {
		var osID shared.ID
		var l linhaItem
		if err := rows.Scan(&osID, &l.Posicao, &l.ServicoID, &l.Descricao, &l.Unidade, &l.Quantidade, &l.ValorUnitario); err != nil {
			return nil, erros.Interno(err)
		}
		porOS[osID] = append(porOS[osID], l.paraDominio())
	}
	return porOS, erros.Interno(rows.Err())
}
```

`shared.ID` é `uuid.UUID` da stdlib (`[16]byte` por baixo): o pgx escaneia e
codifica colunas `uuid` direto, inclusive `[]shared.ID` em `ANY($1)`. Confirmar
no primeiro repositório da fase 0 (teste de round-trip); se não funcionar,
converter para `string` na fronteira, nunca trocar o tipo do domínio.

## Numeração

```go
const sqlProximoNumero = `
INSERT INTO sequencia_documento (empresa_id, tipo, ano, ultimo_numero)
VALUES ($1, 'OS', $2, $3)
ON CONFLICT (empresa_id, tipo, ano)
DO UPDATE SET ultimo_numero = sequencia_documento.ultimo_numero + 1
RETURNING ultimo_numero`
```

`$3` é o número inicial configurado (`OS_NUMERO_INICIAL`), usado só na primeira
linha do ano. O lock de linha do `UPDATE` serializa os concorrentes.

## Leitor: listagem paginada com total

```go
const sqlListarOrdens = `
SELECT id, numero, ano, status, data_emissao, valido_ate, cliente_nome, total_centavos, status_pagamento,
       COUNT(*) OVER() AS total
FROM ordem_servico
WHERE empresa_id = $1
  AND excluido_em IS NULL
  AND ($2 = '' OR status = $2)
  AND ($3 = '' OR unaccent(cliente_nome) ILIKE unaccent('%' || $3 || '%'))
ORDER BY ano DESC, numero DESC
LIMIT $4 OFFSET $5`
```

Filtros opcionais como `($2 = '' OR coluna = $2)` evitam montar SQL por
concatenação. Se a listagem crescer a ponto de o planner sofrer, aí sim
separar em queries específicas.

## Migração — modelo

```sql
-- 002_ordem_servico.sql
-- Ordem de serviço: nasce orçamento e mantém o número. Itens são filhos
-- substituídos em bloco a cada gravação (ver adapter).
CREATE TABLE ordem_servico (
    id                  uuid        PRIMARY KEY,
    empresa_id          uuid        NOT NULL REFERENCES empresa (id),
    numero              integer     NOT NULL,
    ano                 smallint    NOT NULL,
    versao              integer     NOT NULL DEFAULT 1,
    status              text        NOT NULL CHECK (status IN ('ORCAMENTO_ABERTO','ORCAMENTO_ENVIADO','ORCAMENTO_RECUSADO','AGENDADA','EM_ANDAMENTO','CONCLUIDA','CANCELADA')),
    …
    criado_em           timestamptz NOT NULL,
    atualizado_em       timestamptz NOT NULL,
    excluido_em         timestamptz,
    UNIQUE (empresa_id, ano, numero)              -- número nunca se repete no ano
);
-- Listagem principal filtra por status e ordena por número; parcial para ignorar excluídas.
CREATE INDEX ordem_servico_lista_idx ON ordem_servico (empresa_id, status, ano DESC, numero DESC) WHERE excluido_em IS NULL;
```
