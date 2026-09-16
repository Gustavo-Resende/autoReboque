package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Gustavo-Resende/autoSocorro/internal/motorista"
	"github.com/Gustavo-Resende/autoSocorro/internal/ordemservico"
	"github.com/Gustavo-Resende/autoSocorro/internal/servico"
)

// ---------------------------------------------------------------------------
// Serviços: lista e formulário na mesma página.
// ---------------------------------------------------------------------------

func (s *Servidor) servicosLista(w http.ResponseWriter, r *http.Request) {
	s.renderizarServicos(w, r, http.StatusOK, servico.Novo(), "")
}

func (s *Servidor) servicoCriar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	sv := servico.Novo()
	if mensagem := lerFormServico(r, sv); mensagem != "" {
		s.renderizarServicos(w, r, http.StatusUnprocessableEntity, sv, mensagem)
		return
	}
	if err := s.servicos.Criar(r.Context(), sv); err != nil {
		if errors.Is(err, servico.ErrNomeObrigatorio) {
			s.renderizarServicos(w, r, http.StatusUnprocessableEntity, sv, err.Error())
			return
		}
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, "Serviço cadastrado.")
	http.Redirect(w, r, "/servicos", http.StatusSeeOther)
}

func (s *Servidor) servicoEditarForm(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.carregarServico(w, r)
	if !ok {
		return
	}
	s.renderizarServicos(w, r, http.StatusOK, sv, "")
}

func (s *Servidor) servicoAtualizar(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.carregarServico(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	if mensagem := lerFormServico(r, sv); mensagem != "" {
		s.renderizarServicos(w, r, http.StatusUnprocessableEntity, sv, mensagem)
		return
	}
	if err := s.servicos.Atualizar(r.Context(), sv); err != nil {
		if errors.Is(err, servico.ErrNomeObrigatorio) {
			s.renderizarServicos(w, r, http.StatusUnprocessableEntity, sv, err.Error())
			return
		}
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	definirFlash(w, "Serviço salvo.")
	http.Redirect(w, r, "/servicos", http.StatusSeeOther)
}

func (s *Servidor) servicoAtivo(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.carregarServico(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	if err := s.servicos.DefinirAtivo(r.Context(), sv.ID, r.PostForm.Get("ativo") == "1"); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	http.Redirect(w, r, "/servicos", http.StatusSeeOther)
}

func (s *Servidor) carregarServico(w http.ResponseWriter, r *http.Request) (*servico.Servico, bool) {
	id, ok := idDaRota(r, "id")
	if !ok {
		s.erro(w, r, http.StatusNotFound, "Serviço não encontrado.")
		return nil, false
	}
	sv, err := s.servicos.Buscar(r.Context(), id)
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return nil, false
	}
	return sv, true
}

// lerFormServico preenche o serviço; devolve mensagem de erro de conversão, se houver.
func lerFormServico(r *http.Request, sv *servico.Servico) string {
	sv.Nome = limpar(r.PostForm.Get("nome"))
	sv.Unidade = servico.Unidade(r.PostForm.Get("unidade"))
	sv.QuantidadeDoKm = r.PostForm.Get("quantidade_do_km") == "1"
	if r.PostForm.Has("ativo") {
		sv.Ativo = r.PostForm.Get("ativo") == "1"
	}
	preco, err := ordemservico.ParseCentavos(r.PostForm.Get("preco"))
	if err != nil || preco < 0 {
		return "Preço inválido."
	}
	sv.PrecoCentavos = preco
	if o := r.PostForm.Get("ordem"); o != "" {
		n, err := strconv.Atoi(o)
		if err != nil || n < 0 {
			return "Ordem deve ser um número inteiro."
		}
		sv.Ordem = n
	}
	return ""
}

func (s *Servidor) renderizarServicos(w http.ResponseWriter, r *http.Request, status int, emEdicao *servico.Servico, mensagemErro string) {
	lista, err := s.servicos.Listar(r.Context(), false)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := s.base(w, r, "Serviços", "servicos")
	d["Servicos"] = lista
	d["Form"] = emEdicao
	d["Erro"] = mensagemErro
	d["Unidades"] = servico.TodasUnidades
	if err := s.tpl.Renderizar(w, status, "pages/servicos.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

// ---------------------------------------------------------------------------
// Motoristas: mesma ideia, ainda mais simples.
// ---------------------------------------------------------------------------

func (s *Servidor) motoristasLista(w http.ResponseWriter, r *http.Request) {
	s.renderizarMotoristas(w, r, http.StatusOK, &motorista.Motorista{Ativo: true}, "")
}

func (s *Servidor) motoristaCriar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	m := &motorista.Motorista{Nome: limpar(r.PostForm.Get("nome")), Telefone: limpar(r.PostForm.Get("telefone"))}
	if err := s.motoristas.Criar(r.Context(), m); err != nil {
		if errors.Is(err, motorista.ErrNomeObrigatorio) {
			s.renderizarMotoristas(w, r, http.StatusUnprocessableEntity, m, err.Error())
			return
		}
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, "Motorista cadastrado.")
	http.Redirect(w, r, "/motoristas", http.StatusSeeOther)
}

func (s *Servidor) motoristaEditarForm(w http.ResponseWriter, r *http.Request) {
	m, ok := s.carregarMotorista(w, r)
	if !ok {
		return
	}
	s.renderizarMotoristas(w, r, http.StatusOK, m, "")
}

func (s *Servidor) motoristaAtualizar(w http.ResponseWriter, r *http.Request) {
	m, ok := s.carregarMotorista(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	m.Nome = limpar(r.PostForm.Get("nome"))
	m.Telefone = limpar(r.PostForm.Get("telefone"))
	if r.PostForm.Has("ativo") {
		m.Ativo = r.PostForm.Get("ativo") == "1"
	}
	if err := s.motoristas.Atualizar(r.Context(), m); err != nil {
		if errors.Is(err, motorista.ErrNomeObrigatorio) {
			s.renderizarMotoristas(w, r, http.StatusUnprocessableEntity, m, err.Error())
			return
		}
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	definirFlash(w, "Motorista salvo.")
	http.Redirect(w, r, "/motoristas", http.StatusSeeOther)
}

func (s *Servidor) motoristaAtivo(w http.ResponseWriter, r *http.Request) {
	m, ok := s.carregarMotorista(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	if err := s.motoristas.DefinirAtivo(r.Context(), m.ID, r.PostForm.Get("ativo") == "1"); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	http.Redirect(w, r, "/motoristas", http.StatusSeeOther)
}

func (s *Servidor) carregarMotorista(w http.ResponseWriter, r *http.Request) (*motorista.Motorista, bool) {
	id, ok := idDaRota(r, "id")
	if !ok {
		s.erro(w, r, http.StatusNotFound, "Motorista não encontrado.")
		return nil, false
	}
	m, err := s.motoristas.Buscar(r.Context(), id)
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return nil, false
	}
	return m, true
}

func (s *Servidor) renderizarMotoristas(w http.ResponseWriter, r *http.Request, status int, emEdicao *motorista.Motorista, mensagemErro string) {
	lista, err := s.motoristas.Listar(r.Context(), false)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := s.base(w, r, "Motoristas", "motoristas")
	d["Motoristas"] = lista
	d["Form"] = emEdicao
	d["Erro"] = mensagemErro
	if err := s.tpl.Renderizar(w, status, "pages/motoristas.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}
