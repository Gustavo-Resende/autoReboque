package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Gustavo-Resende/autoReboque/internal/cliente"
	"github.com/Gustavo-Resende/autoReboque/internal/ordemservico"
)

func (s *Servidor) clientesLista(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filtro := cliente.Filtro{
		Busca:           q.Get("q"),
		Categoria:       cliente.Categoria(q.Get("categoria")),
		IncluirInativos: q.Get("inativos") == "1",
	}
	filtro.Pagina, _ = strconv.Atoi(q.Get("pagina"))
	if filtro.Categoria != "" && !filtro.Categoria.Valida() {
		filtro.Categoria = ""
	}
	lista, err := s.clientes.Listar(r.Context(), filtro)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := s.base(w, r, "Clientes", "clientes")
	d["Filtro"] = filtro
	d["Lista"] = lista
	d["Categorias"] = cliente.TodasCategorias
	if ehHTMX(r) {
		err = s.tpl.RenderizarBloco(w, http.StatusOK, "pages/clientes_lista.html", "clientes_tabela", d)
	} else {
		err = s.tpl.Renderizar(w, http.StatusOK, "pages/clientes_lista.html", d)
	}
	if err != nil {
		s.erroInterno(w, r, err)
	}
}

// clientesBusca é a lista de sugestões do formulário de OS (HTMX).
func (s *Servidor) clientesBusca(w http.ResponseWriter, r *http.Request) {
	sugestoes, err := s.clientes.Sugerir(r.Context(), r.URL.Query().Get("q"), 8)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := dados{"Sugestoes": sugestoes, "Busca": r.URL.Query().Get("q")}
	if err := s.tpl.RenderizarBloco(w, http.StatusOK, "pages/os_form.html", "clientes_sugestoes", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

func (s *Servidor) clienteNovoForm(w http.ResponseWriter, r *http.Request) {
	s.renderizarFormCliente(w, r, http.StatusOK, cliente.Novo(), "")
}

func (s *Servidor) clienteCriar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	c := cliente.Novo()
	lerFormCliente(r, c)
	if err := s.clientes.Criar(r.Context(), c); err != nil {
		if mensagem, ok := erroDeCliente(err); ok {
			s.renderizarFormCliente(w, r, http.StatusUnprocessableEntity, c, mensagem)
			return
		}
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, "Cliente cadastrado.")
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d", c.ID), http.StatusSeeOther)
}

func (s *Servidor) clienteDetalhe(w http.ResponseWriter, r *http.Request) {
	c, ok := s.carregarCliente(w, r)
	if !ok {
		return
	}
	historico, err := s.ordens.Listar(r.Context(), ordemservico.Filtro{ClienteID: c.ID, ItensPorPagina: 100})
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := s.base(w, r, c.Nome, "clientes")
	d["Cliente"] = c
	d["Historico"] = historico
	if err := s.tpl.Renderizar(w, http.StatusOK, "pages/cliente_detalhe.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

func (s *Servidor) clienteEditarForm(w http.ResponseWriter, r *http.Request) {
	c, ok := s.carregarCliente(w, r)
	if !ok {
		return
	}
	if c.Sistema {
		s.erro(w, r, http.StatusForbidden, "O cliente padrão do sistema não pode ser editado.")
		return
	}
	s.renderizarFormCliente(w, r, http.StatusOK, c, "")
}

func (s *Servidor) clienteAtualizar(w http.ResponseWriter, r *http.Request) {
	c, ok := s.carregarCliente(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	lerFormCliente(r, c)
	if err := s.clientes.Atualizar(r.Context(), c); err != nil {
		if mensagem, ok := erroDeCliente(err); ok {
			s.renderizarFormCliente(w, r, http.StatusUnprocessableEntity, c, mensagem)
			return
		}
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	definirFlash(w, "Cliente salvo.")
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d", c.ID), http.StatusSeeOther)
}

// clienteAtivo desativa ou reativa (campo ativo=1|0).
func (s *Servidor) clienteAtivo(w http.ResponseWriter, r *http.Request) {
	c, ok := s.carregarCliente(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	ativo := r.PostForm.Get("ativo") == "1"
	if err := s.clientes.DefinirAtivo(r.Context(), c.ID, ativo); err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	if ativo {
		definirFlash(w, "Cliente reativado.")
	} else {
		definirFlash(w, "Cliente desativado. O histórico de OS continua disponível.")
	}
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d", c.ID), http.StatusSeeOther)
}

func (s *Servidor) carregarCliente(w http.ResponseWriter, r *http.Request) (*cliente.Cliente, bool) {
	id, ok := idDaRota(r, "id")
	if !ok {
		s.erro(w, r, http.StatusNotFound, "Cliente não encontrado.")
		return nil, false
	}
	c, err := s.clientes.Buscar(r.Context(), id)
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return nil, false
	}
	return c, true
}

func lerFormCliente(r *http.Request, c *cliente.Cliente) {
	campo := func(nome string) string { return limpar(r.PostForm.Get(nome)) }
	c.Tipo = cliente.Tipo(campo("tipo"))
	c.Nome = campo("nome")
	c.Documento = campo("documento")
	c.Telefone = campo("telefone")
	c.Email = campo("email")
	c.Endereco = campo("endereco")
	c.Cidade = campo("cidade")
	c.Categoria = cliente.Categoria(campo("categoria"))
	c.Observacoes = campo("observacoes")
	if r.PostForm.Has("ativo") {
		c.Ativo = r.PostForm.Get("ativo") == "1"
	}
}

// erroDeCliente devolve a mensagem para erros que a pessoa consegue corrigir.
func erroDeCliente(err error) (string, bool) {
	if errors.Is(err, cliente.ErrDocumentoDuplicado) || errors.Is(err, cliente.ErrNomeObrigatorio) ||
		errors.Is(err, cliente.ErrClienteDeSistema) {
		return err.Error(), true
	}
	return "", false
}

func (s *Servidor) renderizarFormCliente(w http.ResponseWriter, r *http.Request, status int, c *cliente.Cliente, mensagemErro string) {
	titulo := "Novo cliente"
	if c.ID != 0 {
		titulo = "Editar cliente"
	}
	d := s.base(w, r, titulo, "clientes")
	d["Cliente"] = c
	d["Erro"] = mensagemErro
	d["Categorias"] = cliente.TodasCategorias
	if err := s.tpl.Renderizar(w, status, "pages/cliente_form.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}
