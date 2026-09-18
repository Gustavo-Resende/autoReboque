---
name: criar-dominio
description: Cria ou altera um pacote de domínio em internal/domain (agregado, entidade, value object, máquina de status, erros de domínio, port de repositório) seguindo o padrão do autoReboque — EntidadeBase com auditoria, ID uuid v7, guard clauses, erros em erros.go, testes puros. Use sempre que o pedido envolver "domínio", "entidade", "agregado", "regra de negócio", "value object", "status", "transição", "invariante" ou "port/interface de repositório", mesmo que o usuário só diga "cria a parte de X" ou "adiciona o cadastro de Y".
---

# Criar domínio

O domínio é o centro do hexágono: só regras de negócio, só stdlib + `pkg/`.
Nada de HTTP, SQL, `time.Now()`, `uuid.NewV7()` fora de `shared`, ou I/O. Um
pacote de domínio bem feito é testável sem subir nada.

## Antes de escrever

1. **Regras primeiro.** Liste as invariantes e transições em uma frase cada
   ("OS só conclui se estiver em andamento"). Se o módulo já existe no legado
   (`internal/<modulo>` antigo), leia-o para extrair as regras — ele é a fonte
   de verdade do comportamento, não da estrutura.
2. **Um agregado por pacote.** O nome do pacote é o agregado no singular, sem
   acento: `ordemservico`, `cliente`, `usuario`. Entidades filhas (ex.: `Item`
   da OS) ficam no mesmo pacote e só são alteradas através da raiz.
3. **Confira `internal/domain/shared`** para não recriar o que já existe
   (`EntidadeBase`, `ID`, `Centavos`, `guard`). Os contratos estão em
   [references/shared.md](references/shared.md).

## Arquivos do pacote

```
internal/domain/<agregado>/
  <agregado>.go        raiz do agregado: struct, construtor NovaX(...), métodos que mudam estado
  <vo>.go              um arquivo por value object com regra própria (veiculo.go, trajeto.go)
  status.go            enum + tabela de transições, se houver ciclo de vida
  erros.go             TODOS os erros do pacote, como sentinelas
  repository.go        port (interface) que a aplicação usa; a implementação fica em adapters/postgres
  <agregado>_test.go   testes puros das regras
```

Se o pacote precisa de mais que isso, provavelmente são dois agregados.

## Padrões (o porquê de cada um)

**Construtor valida; a struct nunca nasce inválida.** Quem recebe um
`*Cliente` pode confiar nele, então as validações não se espalham pelo sistema.

```go
func NovoCliente(empresaID shared.ID, nome, telefone string, agora time.Time) (*Cliente, error) {
	if err := guard.Juntar(
		guard.NaoVazio("nome", nome),
		guard.TamanhoMax("nome", nome, 120),
	); err != nil {
		return nil, err
	}
	return &Cliente{
		EntidadeBase: shared.NovaEntidadeBase(agora),
		EmpresaID:    empresaID,
		Nome:         strings.TrimSpace(nome),
		Telefone:     telefone,
		Ativo:        true,
	}, nil
}
```

**Guard clauses no topo, retorno cedo.** O caminho feliz fica sem indentação
e as pré-condições ficam visíveis de uma olhada. `guard.Juntar` acumula os
erros de campo para o usuário corrigir tudo de uma vez.

**Métodos que mudam estado devolvem `error` e recebem `agora`.** O domínio não
chama `time.Now()`: o handler de aplicação passa o instante (vem do
`port.Clock`), o que deixa os testes determinísticos e a auditoria consistente
(mesmo `agora` para `AtualizadoEm` e para o carimbo da regra).

```go
func (c *Cliente) Desativar(agora time.Time) error {
	if !c.Ativo {
		return ErrJaInativo
	}
	if c.Sistema {
		return ErrClienteDeSistema
	}
	c.Ativo = false
	c.Tocar(agora)
	return nil
}
```

**Erros só em `erros.go`.** Cada erro tem um código estável
(`"<agregado>.<motivo>"`) que o frontend pode usar para mensagens próprias,
e o `Kind` certo para o adapter HTTP escolher o status sem `if`s:

```go
package cliente

import "github.com/Gustavo-Resende/autoReboque/pkg/erros"

var (
	ErrJaInativo        = erros.Regra("cliente.ja_inativo", "o cliente já está inativo")
	ErrClienteDeSistema = erros.Regra("cliente.de_sistema", "o cliente padrão do sistema não pode ser alterado")
	ErrNaoEncontrado    = erros.NaoEncontrado("cliente.nao_encontrado", "cliente não encontrado")
	ErrDocumentoEmUso   = erros.Conflito("cliente.documento_em_uso", "já existe cliente com este documento")
)
```

Quando a mensagem precisa de dados, derive do sentinela para `errors.Is`
continuar funcionando: `ErrTransicaoInvalida.Comf("de %s para %s", de, para)`.

**Máquina de status como tabela.** Uma `map[Status][]Status` de transições +
`PodeIrPara` é mais fácil de ler, testar e listar na UI do que `switch`es
espalhados. Veja a seção "status.go" de [references/exemplo_agregado.md](references/exemplo_agregado.md) (adaptado do legado, que já
tem as transições da OS corretas).

**Port de repositório mínimo e nomeado pelo uso.** Só os métodos que os casos
de uso precisam hoje; nomes de negócio, não CRUD genérico:

```go
type Repository interface {
	Salvar(ctx context.Context, c *Cliente) error                                   // insert ou update; erros.Conflito se a versão mudou
	ObterPorID(ctx context.Context, empresaID, id shared.ID) (*Cliente, error)      // ErrNaoEncontrado
	ExisteDocumento(ctx context.Context, empresaID shared.ID, documento string, exceto shared.ID) (bool, error)
}
```

Listagens com filtro e paginação **não** entram no port do agregado: são
queries (read model) da camada de aplicação — ver `/criar-caso-de-uso`.

**Dinheiro e quantidade são inteiros** (`shared.Centavos`, centésimos). Se
aparecer `float64` em regra de negócio, está errado.

## Testes

Tabela de casos, sem banco, sem mocks de biblioteca. Um teste por regra, nome
que diz a regra, `errors.Is` para o erro esperado:

```go
func TestCliente_NaoDesativaClienteDeSistema(t *testing.T) {
	c := clienteDeSistema(t)
	err := c.Desativar(agora)
	if !errors.Is(err, cliente.ErrClienteDeSistema) {
		t.Fatalf("esperava ErrClienteDeSistema, veio %v", err)
	}
}
```

Helpers de teste (`clienteValido(t)`) ficam no `_test.go` do pacote, com
`t.Helper()`. Usar `agora := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)`
fixo.

## Checklist antes de fechar

- [ ] `go list -deps ./internal/domain/<agregado> | grep -E "adapters|application|pgx|chi"` vazio
- [ ] Construtor e todos os métodos mutáveis validam e devolvem `error`
- [ ] Nenhum `errors.New`/`fmt.Errorf` criando erro de negócio fora de `erros.go`
- [ ] Nenhum `time.Now()`, `uuid.NewV7()` (use `shared.NovoID` só em `shared`/construtor) ou I/O
- [ ] `EntidadeBase` embutida; `Tocar(agora)` chamado em toda mutação
- [ ] Port com só o que os casos de uso usam; doc de cada método diz que erro devolve
- [ ] Testes cobrem cada regra e cada erro
- [ ] `gofmt`, `go vet`, `go test ./internal/domain/...` limpos
- [ ] Commit separado: `feat(dominio): agregado cliente com regras de ativação` (ver `/commits`)

Para o exemplo completo de um agregado com status e itens, leia
[references/exemplo_agregado.md](references/exemplo_agregado.md).
