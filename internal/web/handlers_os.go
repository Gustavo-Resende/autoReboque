package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/cliente"
	"github.com/Gustavo-Resende/autoReboque/internal/ordemservico"
)

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func (s *Servidor) dashboard(w http.ResponseWriter, r *http.Request) {
	// ?mes=2026-09 escolhe o período; sem ele, o mês atual.
	referencia := time.Now()
	if m := r.URL.Query().Get("mes"); m != "" {
		if t, err := time.Parse("2006-01", m); err == nil {
			referencia = t
		}
	}
	periodo := ordemservico.MesDe(referencia)

	ind, err := s.ordens.Dashboard(r.Context(), periodo)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	aguardando, err := s.ordens.Listar(r.Context(), ordemservico.Filtro{Fase: ordemservico.FaseOrcamento, SomenteAguardando: true, ItensPorPagina: 6})
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	recentes, err := s.ordens.Listar(r.Context(), ordemservico.Filtro{Fase: ordemservico.FaseOS, ItensPorPagina: 8})
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}

	d := s.base(w, r, "Visão geral", "dashboard")
	d["Ind"] = ind
	d["Aguardando"] = aguardando.Linhas
	d["Recentes"] = recentes.Linhas
	d["MesAnterior"] = periodo.De.AddDate(0, -1, 0)
	d["MesSeguinte"] = periodo.De.AddDate(0, 1, 0)
	d["EhMesAtual"] = periodo.De.Equal(ordemservico.MesDe(time.Now()).De)
	if err := s.tpl.Renderizar(w, http.StatusOK, "pages/dashboard.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

// ---------------------------------------------------------------------------
// Listas: ordens de serviço e orçamentos são o mesmo registro em fases diferentes
// ---------------------------------------------------------------------------

func (s *Servidor) osLista(w http.ResponseWriter, r *http.Request) {
	s.listar(w, r, ordemservico.FaseOS)
}

func (s *Servidor) orcamentosLista(w http.ResponseWriter, r *http.Request) {
	s.listar(w, r, ordemservico.FaseOrcamento)
}

// listar é a tela de lista: abas por status, filtros e totais.
// Quando o HTMX pede (busca, paginação), devolve só a tabela.
func (s *Servidor) listar(w http.ResponseWriter, r *http.Request, fase ordemservico.Fase) {
	q := r.URL.Query()
	filtro := ordemservico.Filtro{
		Fase:            fase,
		Status:          ordemservico.Status(q.Get("status")),
		StatusPagamento: ordemservico.StatusPagamento(q.Get("pagamento")),
		Busca:           q.Get("q"),
	}
	filtro.Pagina, _ = strconv.Atoi(q.Get("pagina"))
	filtro.ClienteID, _ = strconv.ParseInt(q.Get("cliente"), 10, 64)

	abas := ordemservico.StatusDeOS
	if fase == ordemservico.FaseOrcamento {
		abas = ordemservico.StatusDeOrcamento
	}
	if filtro.Status != "" && !contemStatus(abas, filtro.Status) {
		filtro.Status = ""
	}
	if filtro.StatusPagamento != "" && !filtro.StatusPagamento.Valido() {
		filtro.StatusPagamento = ""
	}

	lista, err := s.ordens.Listar(r.Context(), filtro)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	contagem, err := s.ordens.ContarPorStatus(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	totalFase := 0
	for _, st := range abas {
		totalFase += contagem[st]
	}

	titulo, secao, caminho := "Ordens de serviço", "os", "/os"
	if fase == ordemservico.FaseOrcamento {
		titulo, secao, caminho = "Orçamentos", "orcamentos", "/orcamentos"
	}
	d := s.base(w, r, titulo, secao)
	d["Fase"] = fase
	d["Caminho"] = caminho
	d["Filtro"] = filtro
	d["Lista"] = lista
	d["Contagem"] = contagem
	d["TotalFase"] = totalFase
	d["Abas"] = abas
	d["StatusPagamento"] = ordemservico.TodosStatusPagamento
	d["URLPagina"] = func(p int) string { return urlLista(caminho, filtro, p) }

	if ehHTMX(r) {
		// Atualiza a URL do navegador para a lista filtrada continuar compartilhável.
		w.Header().Set("HX-Push-Url", urlLista(caminho, filtro, filtro.Pagina))
		err = s.tpl.RenderizarBloco(w, http.StatusOK, "pages/os_lista.html", "os_lista_tabela", d)
	} else {
		err = s.tpl.Renderizar(w, http.StatusOK, "pages/os_lista.html", d)
	}
	if err != nil {
		s.erroInterno(w, r, err)
	}
}

func contemStatus(lista []ordemservico.Status, s ordemservico.Status) bool {
	for _, o := range lista {
		if o == s {
			return true
		}
	}
	return false
}

func urlLista(caminho string, f ordemservico.Filtro, pagina int) string {
	v := url.Values{}
	if f.Status != "" {
		v.Set("status", string(f.Status))
	}
	if f.StatusPagamento != "" {
		v.Set("pagamento", string(f.StatusPagamento))
	}
	if f.Busca != "" {
		v.Set("q", f.Busca)
	}
	if f.ClienteID != 0 {
		v.Set("cliente", strconv.FormatInt(f.ClienteID, 10))
	}
	if pagina > 1 {
		v.Set("pagina", strconv.Itoa(pagina))
	}
	if len(v) == 0 {
		return caminho
	}
	return caminho + "?" + v.Encode()
}

// ---------------------------------------------------------------------------
// Criar e editar
// ---------------------------------------------------------------------------

func (s *Servidor) osNovaForm(w http.ResponseWriter, r *http.Request) {
	emp, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	f := formDaOS(ordemservico.Nova(emp.OrcamentoValidadeDias))
	f.ClienteID = 0 // a pessoa escolhe

	// ?cliente=ID pré-seleciona (vindo da tela do cliente).
	if id, err := strconv.ParseInt(r.URL.Query().Get("cliente"), 10, 64); err == nil && id > 0 {
		if c, err := s.clientes.Buscar(r.Context(), id); err == nil {
			f.ClienteID, f.ClienteNome, f.ClienteDocumento, f.ClienteTelefone = c.ID, c.Nome, c.Documento, c.Telefone
		}
	}
	s.renderizarFormOS(w, r, http.StatusOK, f, nil, false)
}

func (s *Servidor) osCriar(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	emp, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	f := formDaRequisicao(r)
	os := ordemservico.Nova(emp.OrcamentoValidadeDias)

	if !s.resolverCliente(r, &f, os) || !s.resolverMotorista(r, &f, os) || !f.aplicar(os) {
		s.renderizarFormOS(w, r, http.StatusUnprocessableEntity, f, nil, false)
		return
	}
	if err := s.ordens.Criar(r.Context(), os); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	definirFlash(w, fmt.Sprintf("Orçamento %s criado.", os.Numero))
	http.Redirect(w, r, fmt.Sprintf("/os/%d", os.ID), http.StatusSeeOther)
}

func (s *Servidor) osEditarForm(w http.ResponseWriter, r *http.Request) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	s.renderizarFormOS(w, r, http.StatusOK, formDaOS(os), os, false)
}

// osAtualizar grava a edição. A versão vem escondida no formulário: se outra
// pessoa salvou no meio tempo, o repositório recusa e avisamos sem sobrescrever.
func (s *Servidor) osAtualizar(w http.ResponseWriter, r *http.Request) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	f := formDaRequisicao(r)
	f.ID = os.ID
	// A OS carregada tem a versão atual do banco; trocamos pela que o
	// formulário trouxe, que é a que a pessoa viu quando começou a editar.
	os.Versao = f.Versao
	if !s.resolverCliente(r, &f, os) || !s.resolverMotorista(r, &f, os) || !f.aplicar(os) {
		s.renderizarFormOS(w, r, http.StatusUnprocessableEntity, f, os, false)
		return
	}
	err := s.ordens.Atualizar(r.Context(), os)
	if errors.Is(err, ordemservico.ErrConflitoVersao) {
		s.renderizarFormOS(w, r, http.StatusConflict, f, os, true)
		return
	}
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	definirFlash(w, fmt.Sprintf("%s %s salva.", tituloCurto(os), os.Numero))
	http.Redirect(w, r, fmt.Sprintf("/os/%d", os.ID), http.StatusSeeOther)
}

// resolverCliente decide quem é o cliente da OS: o escolhido na busca
// (cliente_id), um criado agora pelo cadastro rápido, ou o "Serviço particular".
// Copia nome/documento/telefone para a OS. Erros vão para f.Erros.
func (s *Servidor) resolverCliente(r *http.Request, f *formOS, os *ordemservico.OS) bool {
	ctx := r.Context()
	if f.ClienteID == 0 && f.NovoClienteNome != "" {
		novo := cliente.Novo()
		novo.Nome = f.NovoClienteNome
		novo.Telefone = f.NovoClienteTelefone
		novo.Documento = f.NovoClienteDocumento
		novo.Tipo = cliente.Tipo(f.NovoClienteTipo)
		if err := s.clientes.Criar(ctx, novo); err != nil {
			if errors.Is(err, cliente.ErrDocumentoDuplicado) || errors.Is(err, cliente.ErrNomeObrigatorio) {
				f.Erros = append(f.Erros, "Cadastro rápido: "+err.Error()+".")
				return false
			}
			f.Erros = append(f.Erros, "Não foi possível cadastrar o cliente.")
			return false
		}
		f.ClienteID = novo.ID
	}
	if f.ClienteID == 0 {
		f.Erros = append(f.Erros, "Escolha um cliente, cadastre um novo ou use \"Serviço particular\".")
		return false
	}
	c, err := s.clientes.Buscar(ctx, f.ClienteID)
	if err != nil {
		f.Erros = append(f.Erros, "Cliente não encontrado.")
		f.ClienteID = 0
		return false
	}
	os.Cliente = ordemservico.Cliente{ID: c.ID, Nome: c.Nome, Documento: c.Documento, Telefone: c.Telefone}
	f.ClienteNome, f.ClienteDocumento, f.ClienteTelefone = c.Nome, c.Documento, c.Telefone
	return true
}

// resolverMotorista copia o nome do motorista escolhido (ou aceita texto livre).
func (s *Servidor) resolverMotorista(r *http.Request, f *formOS, os *ordemservico.OS) bool {
	if f.MotoristaID == 0 {
		os.Motorista = ordemservico.Motorista{Nome: f.MotoristaNome}
		return true
	}
	m, err := s.motoristas.Buscar(r.Context(), f.MotoristaID)
	if err != nil {
		f.Erros = append(f.Erros, "Motorista não encontrado.")
		return false
	}
	os.Motorista = ordemservico.Motorista{ID: m.ID, Nome: m.Nome}
	f.MotoristaNome = m.Nome
	return true
}

// renderizarFormOS mostra o formulário de criação/edição.
// os é nil na criação; conflito liga o aviso de "alterada por outra pessoa".
func (s *Servidor) renderizarFormOS(w http.ResponseWriter, r *http.Request, status int, f formOS, os *ordemservico.OS, conflito bool) {
	servicos, err := s.servicos.Listar(r.Context(), true)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	motoristas, err := s.motoristas.Listar(r.Context(), true)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	titulo, secao := "Novo orçamento", "orcamentos"
	if os != nil {
		titulo = "Editar " + os.Numero.String()
		if !os.EhOrcamento() {
			secao = "os"
		}
	}
	d := s.base(w, r, titulo, secao)
	d["Form"] = f
	d["OS"] = os
	d["Conflito"] = conflito
	d["Servicos"] = servicos
	d["Motoristas"] = motoristas
	d["Categorias"] = ordemservico.TodasCategoriasVeiculo
	d["ParticularID"] = cliente.ParticularID
	if err := s.tpl.Renderizar(w, status, "pages/os_form.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

// osItemLinha devolve uma linha de item para o HTMX inserir na tabela.
// Com ?servico=ID, vem preenchida a partir do catálogo; a quantidade de um
// serviço "por km" nasce com o km informado (?km=).
func (s *Servidor) osItemLinha(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	item := formItem{Unidade: "UN", Quantidade: "1"}
	if id, err := strconv.ParseInt(q.Get("servico"), 10, 64); err == nil && id > 0 {
		sv, err := s.servicos.Buscar(r.Context(), id)
		if err == nil {
			item.ServicoID = sv.ID
			item.Descricao = sv.Nome
			item.Unidade = string(sv.Unidade)
			item.ValorUnitario = ordemservico.FormatarValor(sv.PrecoCentavos)
			item.DoKm = sv.QuantidadeDoKm
			if sv.QuantidadeDoKm {
				item.Quantidade = strings.TrimSpace(q.Get("km"))
				if item.Quantidade == "" || item.Quantidade == "0" {
					item.Quantidade = ""
				}
			}
		}
	} else {
		item.Quantidade = ""
	}
	if err := s.tpl.RenderizarBloco(w, http.StatusOK, "pages/os_form.html", "os_item_linha", item); err != nil {
		s.erroInterno(w, r, err)
	}
}

// ---------------------------------------------------------------------------
// Detalhe e ações rápidas
// ---------------------------------------------------------------------------

func (s *Servidor) osDetalhe(w http.ResponseWriter, r *http.Request) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	imagens, err := s.imagens.ListarDaOS(r.Context(), os.ID)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	secao := "os"
	if os.EhOrcamento() {
		secao = "orcamentos"
	}
	d := s.base(w, r, tituloCurto(os)+" "+os.Numero.String(), secao)
	d["OS"] = os
	d["Imagens"] = imagens
	d["StatusPagamento"] = ordemservico.TodosStatusPagamento
	d["Formas"] = ordemservico.TodasFormasPagamento
	d["Etapas"] = etapasDeFoto()
	d["WhatsApp"] = s.linkWhatsApp(r, os)
	if err := s.tpl.Renderizar(w, http.StatusOK, "pages/os_detalhe.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

// osMudarStatus aplica uma transição (botões da tela de detalhe).
// Para "agendar", o formulário manda também agendada_para.
func (s *Servidor) osMudarStatus(w http.ResponseWriter, r *http.Request) {
	s.alterarOS(w, r, func(os *ordemservico.OS, validadeDias int) error {
		novo := ordemservico.Status(r.PostForm.Get("status"))
		if novo == ordemservico.Agendada {
			t, err := time.ParseInLocation("2006-01-02T15:04", r.PostForm.Get("agendada_para"), time.Local)
			if err != nil {
				return errors.New("informe a data e hora do agendamento")
			}
			os.AgendadaPara = t
		}
		return os.MudarStatus(novo, validadeDias)
	})
}

// osPagamento registra valor pago, forma, vencimento e status de pagamento.
func (s *Servidor) osPagamento(w http.ResponseWriter, r *http.Request) {
	s.alterarOS(w, r, func(os *ordemservico.OS, _ int) error {
		valor, err := ordemservico.ParseCentavos(r.PostForm.Get("valor_pago"))
		if err != nil {
			return errors.New("valor pago inválido")
		}
		p := ordemservico.Pagamento{
			Status:        ordemservico.StatusPagamento(r.PostForm.Get("status_pagamento")),
			ValorCentavos: valor,
			Forma:         ordemservico.FormaPagamento(r.PostForm.Get("forma_pagamento")),
		}
		if v := r.PostForm.Get("vencimento"); v != "" && p.Forma == ordemservico.Faturado {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return errors.New("vencimento inválido")
			}
			p.Vencimento = t
		}
		return os.RegistrarPagamento(p)
	})
}

// alterarOS é o esqueleto comum das ações rápidas da tela de detalhe:
// carrega, confere a versão, aplica a mudança, grava e responde.
// Para o HTMX devolve só o painel; sem HTMX, redireciona com mensagem.
func (s *Servidor) alterarOS(w http.ResponseWriter, r *http.Request, mudar func(*ordemservico.OS, int) error) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	emp, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}

	versaoVista, _ := strconv.Atoi(r.PostForm.Get("versao"))
	if versaoVista != os.Versao {
		s.responderPainel(w, r, os, http.StatusConflict, "", true)
		return
	}
	if err := mudar(os, emp.OrcamentoValidadeDias); err != nil {
		s.responderPainel(w, r, os, http.StatusUnprocessableEntity, err.Error(), false)
		return
	}
	err = s.ordens.Atualizar(r.Context(), os)
	if errors.Is(err, ordemservico.ErrConflitoVersao) {
		s.responderPainel(w, r, os, http.StatusConflict, "", true)
		return
	}
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return
	}
	s.responderPainel(w, r, os, http.StatusOK, "", false)
}

// responderPainel renderiza o bloco de status + pagamento da tela de detalhe.
func (s *Servidor) responderPainel(w http.ResponseWriter, r *http.Request, os *ordemservico.OS, status int, mensagemErro string, conflito bool) {
	if !ehHTMX(r) {
		switch {
		case conflito:
			definirFlash(w, "Esta OS foi alterada por outra pessoa. Veja os dados atualizados e tente de novo.")
		case mensagemErro != "":
			definirFlash(w, "Não foi possível alterar: "+mensagemErro)
		default:
			definirFlash(w, "Atualizado.")
		}
		http.Redirect(w, r, fmt.Sprintf("/os/%d", os.ID), http.StatusSeeOther)
		return
	}
	if conflito {
		// Recarrega do banco para mostrar o estado real a quem perdeu a corrida.
		if atual, err := s.ordens.Buscar(r.Context(), os.ID); err == nil {
			os = atual
		}
	}
	d := dados{
		"OS":              os,
		"StatusPagamento": ordemservico.TodosStatusPagamento,
		"Formas":          ordemservico.TodasFormasPagamento,
		"WhatsApp":        s.linkWhatsApp(r, os),
		"Conflito":        conflito,
		"Erro":            mensagemErro,
	}
	// Mudou a fase (orçamento → OS)? O título e o menu da página inteira mudam:
	// mais simples mandar o navegador recarregar do que remendar pedaços.
	if status == http.StatusOK && r.PostForm.Get("status") != "" {
		w.Header().Set("HX-Refresh", "true")
	}
	if err := s.tpl.RenderizarBloco(w, status, "pages/os_detalhe.html", "os_painel", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

// carregarOS lê o {id} da rota e busca a OS; responde 404 se não existir.
func (s *Servidor) carregarOS(w http.ResponseWriter, r *http.Request) (*ordemservico.OS, bool) {
	id, ok := idDaRota(r, "id")
	if !ok {
		s.erro(w, r, http.StatusNotFound, "OS não encontrada.")
		return nil, false
	}
	os, err := s.ordens.Buscar(r.Context(), id)
	if err != nil {
		if !s.naoEncontrada(w, r, err) {
			s.erroInterno(w, r, err)
		}
		return nil, false
	}
	return os, true
}

func tituloCurto(os *ordemservico.OS) string {
	if os.EhOrcamento() {
		return "Orçamento"
	}
	return "OS"
}

// linkWhatsApp monta o link "wa.me" com um resumo do orçamento/OS para o
// cliente. Só existe se o cliente tem telefone. O texto é curto de propósito:
// o documento completo vai como PDF (Ctrl+P → salvar como PDF).
func (s *Servidor) linkWhatsApp(r *http.Request, os *ordemservico.OS) string {
	tel := cliente.SomenteDigitos(os.Cliente.Telefone)
	if len(tel) < 10 {
		return ""
	}
	if len(tel) <= 11 {
		tel = "55" + tel
	}
	emp, err := s.empresa.Buscar(r.Context())
	if err != nil {
		return ""
	}
	var b strings.Builder
	primeiroNome := strings.Fields(os.Cliente.Nome)
	if len(primeiroNome) > 0 && os.Cliente.ID != cliente.ParticularID {
		fmt.Fprintf(&b, "Olá, %s! ", primeiroNome[0])
	} else {
		b.WriteString("Olá! ")
	}
	if os.EhOrcamento() {
		fmt.Fprintf(&b, "Segue o orçamento nº %s da %s:\n", os.Numero, emp.NomeExibicao())
	} else {
		fmt.Fprintf(&b, "Segue a ordem de serviço nº %s da %s:\n", os.Numero, emp.NomeExibicao())
	}
	if os.Veiculo.Modelo != "" || os.Veiculo.Placa != "" {
		fmt.Fprintf(&b, "Veículo: %s %s\n", os.Veiculo.Modelo, os.Veiculo.Placa)
	}
	if os.Trajeto.Origem != "" || os.Trajeto.Destino != "" {
		fmt.Fprintf(&b, "Trajeto: %s → %s\n", os.Trajeto.Origem, os.Trajeto.Destino)
	}
	for _, it := range os.Itens {
		fmt.Fprintf(&b, "• %s: %s\n", it.Descricao, ordemservico.FormatarReais(it.Subtotal()))
	}
	if os.DescontoCentavos > 0 {
		fmt.Fprintf(&b, "• Desconto: -%s\n", ordemservico.FormatarReais(os.DescontoCentavos))
	}
	fmt.Fprintf(&b, "*Total: %s*\n", ordemservico.FormatarReais(os.Total()))
	if os.EhOrcamento() && !os.ValidoAte.IsZero() {
		fmt.Fprintf(&b, "Válido até %s.\n", os.ValidoAte.Format("02/01/2006"))
	}
	b.WriteString("Qualquer dúvida, estamos à disposição.")
	return "https://wa.me/" + tel + "?text=" + url.QueryEscape(b.String())
}
