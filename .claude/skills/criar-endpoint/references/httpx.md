# `pkg/httpx` e `pkg/validar` — contratos

## `pkg/httpx`

```go
package httpx

// Decodificar lê JSON do corpo em T de forma estrita (campos desconhecidos são
// erro, corpo limitado a 1 MB) e, se T implementa Validavel, chama Validar().
// Erros são *erros.Erro de Validacao: JSON inválido vira Campo("corpo", …);
// campo desconhecido vira Campo("<nome>", "campo desconhecido").
func Decodificar[T any](r *http.Request) (T, error)

type Validavel interface{ Validar() error }

// ParamID lê um path param do chi e converte em shared.ID (Campo("id", "id inválido")).
func ParamID(r *http.Request, nome string) (shared.ID, error)

// Query dá acesso tipado à query string com padrão.
func Query(r *http.Request) Params
func (p Params) Int(nome string, padrao int) int
func (p Params) String(nome string) string
func (p Params) Bool(nome string) bool

// Escrever serializa v como JSON com o status dado (Content-Type application/json; charset=utf-8).
func Escrever(w http.ResponseWriter, status int, v any)
func SemConteudo(w http.ResponseWriter) // 204

// EscreverErro mapeia o erro para status e corpo padrão. Interno → 500 com
// mensagem genérica, erro real no log com request id.
func EscreverErro(w http.ResponseWriter, r *http.Request, err error)

type Criado struct {
	ID shared.ID `json:"id"`
}
type Atualizado struct {
	ID     shared.ID `json:"id"`
	Versao int       `json:"versao"`
}

// Sessão autenticada, colocada no ctx pelo middleware de autenticação.
type Sessao struct {
	UsuarioID shared.ID
	EmpresaID shared.ID
}
func SessaoDe(ctx context.Context) Sessao          // panic se ausente: rota protegida sem middleware é bug de wiring
func ComSessao(ctx context.Context, s Sessao) context.Context
```

### Mapeamento `erros.Kind` → HTTP

| Kind | Status | Corpo |
|---|---|---|
| `Validacao` | 400 | `{"codigo":"validacao","mensagem":"dados inválidos","campos":{"cliente_id":"obrigatório"}}` |
| `NaoAutorizado` | 401 | `{"codigo":"auth.nao_autorizado","mensagem":"…"}` |
| `Proibido` | 403 | |
| `NaoEncontrado` | 404 | `{"codigo":"os.nao_encontrada","mensagem":"ordem de serviço não encontrada"}` |
| `Conflito` | 409 | `{"codigo":"entidade.versao_desatualizada","mensagem":"…"}` |
| `RegraNegocio` | 422 | `{"codigo":"os.transicao_invalida","mensagem":"não é possível ir de Concluída para Agendada"}` |
| `Interno` | 500 | `{"codigo":"interno","mensagem":"erro inesperado; tente de novo"}` |

O `codigo` é estável: o frontend pode mapear para mensagens próprias. A
`mensagem` já vem em pt-BR e pode ser mostrada como está.

## `pkg/validar`

Regras puras que devolvem `error` (`*erros.Erro` de `Validacao` com `Campos`)
ou `nil`. Sem reflexão, sem tags: a validação de cada DTO é código Go legível
e testável.

```go
package validar

func Obrigatorio(campo, valor string) error                 // TrimSpace != ""
func TamanhoMax(campo, valor string, max int) error         // runes
func TamanhoEntre(campo, valor string, min, max int) error
func TamanhoMaxLista(campo string, n, max int) error
func UUID(campo, valor string) error                        // ignora vazio (combine com Obrigatorio)
func Email(campo, valor string) error
func Telefone(campo, valor string) error                    // dígitos BR: 10 ou 11, com ou sem máscara
func Placa(campo, valor string) error                       // ABC1234 ou ABC1D23
func Data(campo, valor string) error                        // "2006-01-02"
func DataHora(campo, valor string) error                    // RFC 3339
func UmDe(campo, valor string, opcoes ...string) error
func Inteiro(campo string, v, min, max int64) error
func Que(cond bool, campo, msg string) error

// Prefixo prefixa os campos de um erro de validação ("itens[2].descricao").
func Prefixo(prefixo string, err error) error
// Juntar funde erros de validação (todos os campos) ou devolve o primeiro erro de outro tipo.
func Juntar(errs ...error) error
```

Mensagens padrão em pt-BR ("obrigatório", "no máximo 120 caracteres",
"telefone inválido"). `domain/guard` usa as mesmas primitivas por baixo
quando faz sentido, mas com a semântica de invariante do domínio.

## `pkg/config`

```go
type Config struct {
	DatabaseURL       string        // obrigatória
	HTTPAddr          string        // ":8080"
	CORSOrigens       []string      // "http://localhost:5173"
	SessaoDuracao     time.Duration // 720h
	OSNumeroInicial   int
	StorageDir        string
	UploadMaxMB       int
	Dev               bool
}
func Carregar() (Config, error) // lê .env se existir (variáveis do ambiente têm prioridade), valida obrigatórias
```
