# Contratos de `internal/domain/shared` e `pkg/erros`

Este é o contrato que a fase 0 do plano implementa e que todo domínio usa.
Se o código real divergir daqui, o código manda — atualize este arquivo.

## `pkg/erros` — taxonomia única de erros

```go
package erros

type Kind uint8

const (
	Interno       Kind = iota // falha inesperada (bug, banco fora) → HTTP 500, mensagem genérica
	Validacao                 // entrada malformada ou campo inválido → 400 (com Campos)
	NaoEncontrado             // recurso não existe (ou está excluído) → 404
	Conflito                  // versão desatualizada, chave duplicada → 409
	RegraNegocio              // operação não permitida pelo estado atual → 422
	NaoAutorizado             // sem credenciais válidas → 401
	Proibido                  // credenciais ok, sem permissão → 403
)

// Erro é o único tipo de erro de negócio do sistema. Sentinelas são *Erro; a
// comparação em errors.Is é pelo Codigo, então derivar com Comf/ComCausa não
// quebra o Is.
type Erro struct {
	Kind     Kind
	Codigo   string            // estável, "agregado.motivo": o frontend depende dele
	Mensagem string            // pt-BR, para a pessoa
	Campos   map[string]string // campo → mensagem, só em Validacao
	causa    error
}

func (e *Erro) Error() string
func (e *Erro) Unwrap() error
func (e *Erro) Is(alvo error) bool                       // mesmo Codigo
func (e *Erro) Comf(formato string, args ...any) *Erro   // cópia com mensagem nova
func (e *Erro) ComCausa(causa error) *Erro               // cópia envolvendo a causa

func Validacao(codigo, msg string) *Erro
func Campo(campo, msg string) *Erro                      // Validacao com um campo
func Campos(campos map[string]string) *Erro              // Validacao com vários
func NaoEncontrado(codigo, msg string) *Erro
func Conflito(codigo, msg string) *Erro
func Regra(codigo, msg string) *Erro
func NaoAutorizado(codigo, msg string) *Erro
func Proibido(codigo, msg string) *Erro
func Interno(causa error) *Erro

func KindDe(err error) Kind   // Interno quando err não é *Erro (nem envolve um)
func Juntar(errs ...error) error // nil se todos nil; funde Campos de erros de Validacao; senão devolve o primeiro
```

## `internal/domain/shared`

```go
package shared

// ID de qualquer entidade. UUID v7 (stdlib do Go 1.27): ordenado no tempo,
// então índices B-tree do Postgres não fragmentam como com v4.
type ID = uuid.UUID

func NovoID() ID                 // uuid.NewV7()
func ParseID(s string) (ID, error) // erros.Campo("id", "id inválido") em falha
var IDVazio ID                   // uuid.Nil()

// EntidadeBase é embutida em toda raiz de agregado e entidade persistida.
// Auditoria de "quando"; o "quem" (CriadoPor/AtualizadoPor) entra quando
// houver usuários individuais.
type EntidadeBase struct {
	ID           ID
	CriadoEm     time.Time
	AtualizadoEm time.Time
	ExcluidoEm   *time.Time // nil = ativa. Exclusão é sempre lógica.
	Versao       int        // optimistic locking; começa em 1, o repositório incrementa ao salvar
}

func NovaEntidadeBase(agora time.Time) EntidadeBase   // ID novo, CriadoEm = AtualizadoEm = agora, Versao 1
func (e *EntidadeBase) Tocar(agora time.Time)         // AtualizadoEm = agora
func (e *EntidadeBase) Excluir(agora time.Time) error // ErrJaExcluida se já excluída; senão marca e Tocar
func (e EntidadeBase) Excluida() bool

var ErrJaExcluida = erros.Regra("entidade.ja_excluida", "o registro já foi excluído")
var ErrVersaoDesatualizada = erros.Conflito("entidade.versao_desatualizada", "o registro foi alterado por outra pessoa; recarregue e tente de novo")

// Centavos é dinheiro. Nunca float. Formatação para a pessoa fica no frontend
// (a API devolve o inteiro); Parse existe para o seed e para testes.
type Centavos int64
func (c Centavos) String() string             // "R$ 1.234,56"
func ParseCentavos(s string) (Centavos, error) // aceita "1234,56", "1.234,56", "1234.56"

// Centesimos é quantidade com duas casas (1,50 h = 150).
type Centesimos int64
func (q Centesimos) Vezes(valor Centavos) Centavos // arredonda para o centavo mais próximo
```

## `internal/domain/guard` — guard clauses

Funções puras que devolvem `*erros.Erro` de `Validacao` com o campo, ou `nil`.
Usadas no topo de construtores e métodos; `Juntar` acumula.

```go
package guard

func NaoVazio(campo, valor string) error                 // após TrimSpace
func TamanhoMax(campo, valor string, max int) error      // em runes
func TamanhoEntre(campo, valor string, min, max int) error
func Positivo[N ~int | ~int64](campo string, v N) error  // > 0
func NaoNegativo[N ~int | ~int64](campo string, v N) error
func Entre[N ~int | ~int64](campo string, v, min, max N) error
func UmDe[T comparable](campo string, v T, opcoes ...T) error
func Que(cond bool, campo, msg string) error             // condição arbitrária
func Juntar(errs ...error) error                         // = erros.Juntar
```

Regra prática: `guard` valida **forma e limites** dentro do domínio
(invariantes simples). Regras que dependem de estado ("não pode concluir sem
iniciar") são `if` + sentinela de `erros.go`, não guard.
