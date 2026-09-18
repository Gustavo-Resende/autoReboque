package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Gustavo-Resende/autoReboque/internal/empresa"
	"github.com/Gustavo-Resende/autoReboque/internal/imagem"
	"github.com/Gustavo-Resende/autoReboque/internal/storage"
)

func (s *Servidor) empresaForm(w http.ResponseWriter, r *http.Request) {
	e, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	s.renderizarEmpresa(w, r, http.StatusOK, e, "")
}

func (s *Servidor) empresaAtualizar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	e, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	campo := func(nome string) string { return limpar(r.PostForm.Get(nome)) }
	e.NomeFantasia = campo("nome_fantasia")
	e.RazaoSocial = campo("razao_social")
	e.CNPJ = campo("cnpj")
	e.Endereco = campo("endereco")
	e.Cidade = campo("cidade")
	e.Telefone = campo("telefone")
	e.WhatsApp = campo("whatsapp")
	e.Email = campo("email")
	e.CorPrimaria = campo("cor_primaria")
	e.OrcamentoCondicoes = campo("orcamento_condicoes")
	e.OrcamentoValidadeDias, _ = strconv.Atoi(campo("orcamento_validade_dias"))

	if err := s.empresa.Atualizar(r.Context(), e); err != nil {
		// Erros de validação são de texto simples, vindos de Validar().
		s.renderizarEmpresa(w, r, http.StatusUnprocessableEntity, e, err.Error())
		return
	}
	definirFlash(w, "Dados da empresa salvos.")
	http.Redirect(w, r, "/empresa", http.StatusSeeOther)
}

func (s *Servidor) empresaLogoEnviar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.UploadMaxBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.empresaErro(w, r, "Arquivo muito grande.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	arquivo, _, err := r.FormFile("logo")
	if err != nil {
		s.empresaErro(w, r, "Selecione um arquivo de imagem.")
		return
	}
	defer arquivo.Close()

	dados, err := s.imagens.LerLimitado(arquivo)
	if err != nil {
		s.empresaErro(w, r, "Arquivo muito grande.")
		return
	}
	if int64(len(dados)) > 2<<20 {
		s.empresaErro(w, r, "O logo deve ter no máximo 2 MB.")
		return
	}
	formato, err := imagem.Detectar(dados)
	if err != nil {
		s.empresaErro(w, r, err.Error())
		return
	}
	if err := s.empresa.AtualizarLogo(r.Context(), formato.Extensao, bytes.NewReader(dados)); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, "Logo atualizado.")
	http.Redirect(w, r, "/empresa", http.StatusSeeOther)
}

func (s *Servidor) empresaLogoRemover(w http.ResponseWriter, r *http.Request) {
	if err := s.empresa.RemoverLogo(r.Context()); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, "Logo removido.")
	http.Redirect(w, r, "/empresa", http.StatusSeeOther)
}

// empresaLogo serve a imagem do logo (tela, login e impressão).
func (s *Servidor) empresaLogo(w http.ResponseWriter, r *http.Request) {
	conteudo, err := s.empresa.AbrirLogo(r.Context())
	if errors.Is(err, storage.ErrNaoEncontrado) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	defer conteudo.Close()
	dados, err := io.ReadAll(conteudo)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(dados))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(dados)
}

func (s *Servidor) empresaErro(w http.ResponseWriter, r *http.Request, mensagem string) {
	e, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	s.renderizarEmpresa(w, r, http.StatusUnprocessableEntity, e, mensagem)
}

func (s *Servidor) renderizarEmpresa(w http.ResponseWriter, r *http.Request, status int, e *empresa.Empresa, mensagemErro string) {
	d := s.base(w, r, "Empresa", "empresa")
	d["Form"] = e
	d["Erro"] = mensagemErro
	if err := s.tpl.Renderizar(w, status, "pages/empresa.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}
