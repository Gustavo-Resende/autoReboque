package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/cliente"
	"github.com/Gustavo-Resende/autoReboque/internal/ordemservico"
)

// Templates e arquivos estáticos vão embutidos no binário. Em modo DEV,
// os templates são relidos do disco a cada requisição para agilizar o ajuste.
//
//go:embed templates static
var arquivosEmbutidos embed.FS

// Templates carrega e renderiza as páginas.
//
// Cada página é um conjunto próprio: layout.html + partials/*.html + pages/X.html.
// Assim, duas páginas podem definir o bloco "conteudo" sem conflito.
// Os templates de impressão (print/*.html) não usam o layout do sistema.
type Templates struct {
	fsys fs.FS
	dev  bool

	mu    sync.Mutex
	cache map[string]*template.Template
}

func NovosTemplates(dev bool) (*Templates, error) {
	var fsys fs.FS
	if dev {
		fsys = os.DirFS("internal/web/templates")
	} else {
		sub, err := fs.Sub(arquivosEmbutidos, "templates")
		if err != nil {
			return nil, err
		}
		fsys = sub
	}
	t := &Templates{fsys: fsys, dev: dev, cache: map[string]*template.Template{}}
	if !dev {
		// Fora do DEV, compila tudo na subida para falhar cedo se houver erro.
		for _, pagina := range paginas {
			if _, err := t.carregar(pagina); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

// paginas lista os templates que existem, para validar na subida.
var paginas = []string{
	"pages/login.html",
	"pages/erro.html",
	"pages/dashboard.html",
	"pages/os_lista.html",
	"pages/os_form.html",
	"pages/os_detalhe.html",
	"pages/clientes_lista.html",
	"pages/cliente_form.html",
	"pages/cliente_detalhe.html",
	"pages/servicos.html",
	"pages/motoristas.html",
	"pages/empresa.html",
	"print/os.html",
}

var mesesCurtos = []string{"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}

// funcoes são os auxiliares disponíveis dentro dos templates.
var funcoes = template.FuncMap{
	"reais":   ordemservico.FormatarReais,
	"valor":   ordemservico.FormatarValor,
	"qtd":     ordemservico.FormatarQuantidade,
	"unidade": ordemservico.RotuloUnidade,
	"data": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("02/01/2006")
	},
	"dataInput": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	},
	"dataHora": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format("02/01/2006 15:04")
	},
	"dataHoraInput": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format("2006-01-02T15:04")
	},
	"mesCurto": func(t time.Time) string { return mesesCurtos[t.Month()-1] },
	"mesAno":   func(t time.Time) string { return fmt.Sprintf("%s/%d", mesesCurtos[t.Month()-1], t.Year()) },
	"mesInput": func(t time.Time) string { return t.Format("2006-01") },
	"inc":      func(n int) int { return n + 1 },
	"dec":      func(n int) int { return n - 1 },
	// percentual devolve parte/total em 0..100, seguro contra total zero.
	"percentual": func(parte, total int64) int {
		if total <= 0 {
			return 0
		}
		return int(parte * 100 / total)
	},
	"digitos":  cliente.SomenteDigitos,
	"urlquery": url.QueryEscape,
	"lower":    strings.ToLower,
	// dict monta um mapa dentro do template, para passar vários valores a um bloco:
	// {{template "bloco" (dict "Chave" .Valor "Outra" 1)}}
	"dict": func(pares ...any) (map[string]any, error) {
		if len(pares)%2 != 0 {
			return nil, errors.New("dict: número ímpar de argumentos")
		}
		m := make(map[string]any, len(pares)/2)
		for i := 0; i < len(pares); i += 2 {
			chave, ok := pares[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict: chave %v não é texto", pares[i])
			}
			m[chave] = pares[i+1]
		}
		return m, nil
	},
}

func (t *Templates) carregar(pagina string) (*template.Template, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tpl, ok := t.cache[pagina]; ok && !t.dev {
		return tpl, nil
	}

	tpl := template.New(path.Base(pagina)).Funcs(funcoes)
	arquivos := []string{"layout.html"}
	parciais, err := fs.Glob(t.fsys, "partials/*.html")
	if err != nil {
		return nil, err
	}
	arquivos = append(arquivos, parciais...)
	arquivos = append(arquivos, pagina)

	tpl, err = tpl.ParseFS(t.fsys, arquivos...)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", pagina, err)
	}
	t.cache[pagina] = tpl
	return tpl, nil
}

// Renderizar escreve a página inteira (layout + conteúdo) com o status dado.
// Renderiza primeiro em memória: se der erro no meio, a resposta não sai pela metade.
func (t *Templates) Renderizar(w http.ResponseWriter, status int, pagina string, dados any) error {
	return t.executar(w, status, pagina, "layout", dados)
}

// RenderizarBloco escreve só um bloco nomeado (usado nas respostas ao HTMX
// e nos templates de impressão, que definem o próprio bloco raiz).
func (t *Templates) RenderizarBloco(w http.ResponseWriter, status int, pagina, bloco string, dados any) error {
	return t.executar(w, status, pagina, bloco, dados)
}

func (t *Templates) executar(w http.ResponseWriter, status int, pagina, bloco string, dados any) error {
	tpl, err := t.carregar(pagina)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, bloco, dados); err != nil {
		return fmt.Errorf("renderizar %s/%s: %w", pagina, bloco, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = buf.WriteTo(w)
	return err
}
