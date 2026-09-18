// Package web é a camada HTTP: rotas, handlers, templates e arquivos estáticos.
//
// Os handlers traduzem requisições em chamadas ao domínio e o resultado em
// HTML. Nenhuma regra de negócio mora aqui.
package web

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/auth"
	"github.com/Gustavo-Resende/autoReboque/internal/cliente"
	"github.com/Gustavo-Resende/autoReboque/internal/config"
	"github.com/Gustavo-Resende/autoReboque/internal/empresa"
	"github.com/Gustavo-Resende/autoReboque/internal/imagem"
	"github.com/Gustavo-Resende/autoReboque/internal/motorista"
	"github.com/Gustavo-Resende/autoReboque/internal/ordemservico"
	"github.com/Gustavo-Resende/autoReboque/internal/servico"
)

// Servidor junta tudo que os handlers precisam.
type Servidor struct {
	cfg        config.Config
	tpl        *Templates
	ordens     *ordemservico.Repositorio
	clientes   *cliente.Repositorio
	servicos   *servico.Repositorio
	motoristas *motorista.Repositorio
	imagens    *imagem.Servico
	empresa    *empresa.Repositorio
	sessoes    *auth.Sessoes
	verificar  func() error // healthcheck do banco
}

type Dependencias struct {
	Config     config.Config
	Ordens     *ordemservico.Repositorio
	Clientes   *cliente.Repositorio
	Servicos   *servico.Repositorio
	Motoristas *motorista.Repositorio
	Imagens    *imagem.Servico
	Empresa    *empresa.Repositorio
	Sessoes    *auth.Sessoes
	Verificar  func() error
}

func NovoServidor(d Dependencias) (*Servidor, error) {
	tpl, err := NovosTemplates(d.Config.Dev)
	if err != nil {
		return nil, err
	}
	return &Servidor{
		cfg:        d.Config,
		tpl:        tpl,
		ordens:     d.Ordens,
		clientes:   d.Clientes,
		servicos:   d.Servicos,
		motoristas: d.Motoristas,
		imagens:    d.Imagens,
		empresa:    d.Empresa,
		sessoes:    d.Sessoes,
		verificar:  d.Verificar,
	}, nil
}

// Rotas monta o roteador. Padrões "MÉTODO /caminho/{param}" são do
// http.ServeMux da biblioteca padrão (Go 1.22+).
func (s *Servidor) Rotas() http.Handler {
	publico := http.NewServeMux()
	publico.HandleFunc("GET /healthz", s.healthz)
	publico.HandleFunc("GET /login", s.loginForm)
	publico.HandleFunc("POST /login", s.login)
	publico.HandleFunc("GET /empresa/logo", s.empresaLogo) // aparece na tela de login

	estaticos, _ := fs.Sub(arquivosEmbutidos, "static")
	publico.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(estaticos)))

	privado := http.NewServeMux()
	privado.HandleFunc("GET /{$}", s.dashboard)
	privado.HandleFunc("POST /logout", s.logout)

	// Ordens de serviço e orçamentos (mesmo registro, telas diferentes).
	privado.HandleFunc("GET /os", s.osLista)
	privado.HandleFunc("GET /orcamentos", s.orcamentosLista)
	privado.HandleFunc("GET /os/nova", s.osNovaForm)
	privado.HandleFunc("POST /os", s.osCriar)
	privado.HandleFunc("GET /os/item-linha", s.osItemLinha)
	privado.HandleFunc("GET /os/{id}", s.osDetalhe)
	privado.HandleFunc("GET /os/{id}/editar", s.osEditarForm)
	privado.HandleFunc("POST /os/{id}", s.osAtualizar)
	privado.HandleFunc("POST /os/{id}/status", s.osMudarStatus)
	privado.HandleFunc("POST /os/{id}/pagamento", s.osPagamento)
	privado.HandleFunc("GET /os/{id}/imprimir", s.osImprimir)

	privado.HandleFunc("POST /os/{id}/imagens", s.imagemEnviar)
	privado.HandleFunc("POST /os/{id}/imagens/{imagemID}/etapa", s.imagemEtapa)
	privado.HandleFunc("POST /os/{id}/imagens/{imagemID}/remover", s.imagemRemover)
	privado.HandleFunc("GET /imagens/{imagemID}", s.imagemOriginal)
	privado.HandleFunc("GET /imagens/{imagemID}/miniatura", s.imagemMiniatura)

	// Cadastros.
	privado.HandleFunc("GET /clientes", s.clientesLista)
	privado.HandleFunc("GET /clientes/busca", s.clientesBusca)
	privado.HandleFunc("GET /clientes/novo", s.clienteNovoForm)
	privado.HandleFunc("POST /clientes", s.clienteCriar)
	privado.HandleFunc("GET /clientes/{id}", s.clienteDetalhe)
	privado.HandleFunc("GET /clientes/{id}/editar", s.clienteEditarForm)
	privado.HandleFunc("POST /clientes/{id}", s.clienteAtualizar)
	privado.HandleFunc("POST /clientes/{id}/ativo", s.clienteAtivo)

	privado.HandleFunc("GET /servicos", s.servicosLista)
	privado.HandleFunc("POST /servicos", s.servicoCriar)
	privado.HandleFunc("GET /servicos/{id}/editar", s.servicoEditarForm)
	privado.HandleFunc("POST /servicos/{id}", s.servicoAtualizar)
	privado.HandleFunc("POST /servicos/{id}/ativo", s.servicoAtivo)

	privado.HandleFunc("GET /motoristas", s.motoristasLista)
	privado.HandleFunc("POST /motoristas", s.motoristaCriar)
	privado.HandleFunc("GET /motoristas/{id}/editar", s.motoristaEditarForm)
	privado.HandleFunc("POST /motoristas/{id}", s.motoristaAtualizar)
	privado.HandleFunc("POST /motoristas/{id}/ativo", s.motoristaAtivo)

	privado.HandleFunc("GET /empresa", s.empresaForm)
	privado.HandleFunc("POST /empresa", s.empresaAtualizar)
	privado.HandleFunc("POST /empresa/logo", s.empresaLogoEnviar)
	privado.HandleFunc("POST /empresa/logo/remover", s.empresaLogoRemover)

	publico.Handle("/", auth.RequerLogin(s.sessoes, privado))
	return registrarAcesso(publico)
}

func (s *Servidor) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.verificar(); err != nil {
		slog.Error("healthcheck", "erro", err)
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

// registrarAcesso escreve uma linha de log por requisição.
func registrarAcesso(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		gravador := &respostaComStatus{ResponseWriter: w, status: http.StatusOK}
		proximo.ServeHTTP(gravador, r)
		slog.Info("http", "metodo", r.Method, "caminho", r.URL.Path, "status", gravador.status,
			"duracao", time.Since(inicio).Round(time.Millisecond))
	})
}

type respostaComStatus struct {
	http.ResponseWriter
	status int
}

func (r *respostaComStatus) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// ---- auxiliares usados por vários handlers ----

// dados é o mapa passado aos templates. Chaves comuns: Titulo, Flash, Logado, Empresa.
type dados map[string]any

// base monta o mapa com o que toda página precisa: título, empresa (nome, logo
// e cor da marca para o layout), flash e a seção ativa do menu.
func (s *Servidor) base(w http.ResponseWriter, r *http.Request, titulo, secao string) dados {
	d := dados{
		"Titulo": titulo,
		"Logado": auth.TokenDoCookie(r) != "",
		"Secao":  secao,
	}
	if emp, err := s.empresa.Buscar(r.Context()); err == nil {
		d["Empresa"] = emp
	} else {
		slog.Error("ler empresa para o layout", "erro", err)
		d["Empresa"] = &empresa.Empresa{NomeFantasia: "Sistema", CorPrimaria: empresa.CorPadrao}
	}
	if flash := lerFlash(r); flash != "" {
		d["Flash"] = flash
		apagarFlash(w) // a mensagem aparece uma vez só
	}
	return d
}

// ehHTMX diz se a requisição veio do HTMX (e portanto quer só um pedaço da página).
func ehHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// idDaRota lê um parâmetro numérico do caminho, ex.: /os/{id}.
func idDaRota(r *http.Request, nome string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(nome), 10, 64)
	return id, err == nil && id > 0
}

// erro renderiza a página de erro com o status dado.
func (s *Servidor) erro(w http.ResponseWriter, r *http.Request, status int, mensagem string) {
	d := s.base(w, r, "Erro", "")
	d["Status"] = status
	d["Mensagem"] = mensagem
	if err := s.tpl.Renderizar(w, status, "pages/erro.html", d); err != nil {
		slog.Error("renderizar página de erro", "erro", err)
		http.Error(w, mensagem, status)
	}
}

// erroInterno registra o erro e mostra uma mensagem genérica.
func (s *Servidor) erroInterno(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("erro interno", "caminho", r.URL.Path, "erro", err)
	s.erro(w, r, http.StatusInternalServerError, "Ocorreu um erro inesperado. Tente novamente.")
}

// naoEncontrada trata erros de "não existe" do domínio.
func (s *Servidor) naoEncontrada(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, ordemservico.ErrNaoEncontrada) || errors.Is(err, imagem.ErrNaoEncontrada) ||
		errors.Is(err, cliente.ErrNaoEncontrado) || errors.Is(err, servico.ErrNaoEncontrado) ||
		errors.Is(err, motorista.ErrNaoEncontrado) {
		s.erro(w, r, http.StatusNotFound, "Não encontrado.")
		return true
	}
	return false
}

// ---- flash: mensagem curta que sobrevive a um redirect, via cookie ----

const cookieFlash = "flash"

func definirFlash(w http.ResponseWriter, mensagem string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieFlash, Value: mensagem, Path: "/", MaxAge: 60, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// lerFlash devolve a mensagem do cookie, se houver.
func lerFlash(r *http.Request) string {
	c, err := r.Cookie(cookieFlash)
	if err != nil {
		return ""
	}
	return c.Value
}

func apagarFlash(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieFlash, Value: "", Path: "/", MaxAge: -1})
}
