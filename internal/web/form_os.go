package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gustavo-Resende/autoSocorro/internal/ordemservico"
)

// formOS é o formulário de OS como texto: tudo string, do jeito que veio do
// navegador. Só depois de validado vira uma ordemservico.OS. Manter os dois
// separados permite re-exibir exatamente o que a pessoa digitou quando há erro.
type formOS struct {
	ID     int64
	Versao int

	DataEmissao string
	AcionadoEm  string

	// Cliente escolhido (id) e a cópia dos dados para mostrar o "selecionado".
	ClienteID        int64
	ClienteNome      string
	ClienteDocumento string
	ClienteTelefone  string

	// Cadastro rápido: preenchido quando não há cliente escolhido.
	NovoClienteNome      string
	NovoClienteTelefone  string
	NovoClienteDocumento string
	NovoClienteTipo      string

	SolicitanteNome     string
	SolicitanteTelefone string

	VeiculoCategoria string
	VeiculoModelo    string
	VeiculoAno       string
	VeiculoCor       string
	VeiculoPlaca     string
	VeiculoCondicao  string

	TrajetoOrigem     string
	TrajetoReferencia string
	TrajetoDestino    string
	TrajetoKm         string

	MotoristaID   int64
	MotoristaNome string
	Guincho       string
	RecebidoPor   string
	Observacoes   string

	Itens    []formItem
	Desconto string

	Erros []string
}

type formItem struct {
	ServicoID     int64
	Descricao     string
	Unidade       string
	Quantidade    string
	ValorUnitario string
	DoKm          bool // a quantidade acompanha o km do trajeto
}

// formDaOS preenche o formulário a partir de uma OS existente (edição).
func formDaOS(os *ordemservico.OS) formOS {
	f := formOS{
		ID:                  os.ID,
		Versao:              os.Versao,
		DataEmissao:         os.DataEmissao.Format("2006-01-02"),
		ClienteID:           os.Cliente.ID,
		ClienteNome:         os.Cliente.Nome,
		ClienteDocumento:    os.Cliente.Documento,
		ClienteTelefone:     os.Cliente.Telefone,
		NovoClienteTipo:     "PF",
		SolicitanteNome:     os.Solicitante.Nome,
		SolicitanteTelefone: os.Solicitante.Telefone,
		VeiculoCategoria:    string(os.Veiculo.Categoria),
		VeiculoModelo:       os.Veiculo.Modelo,
		VeiculoAno:          os.Veiculo.Ano,
		VeiculoCor:          os.Veiculo.Cor,
		VeiculoPlaca:        os.Veiculo.Placa,
		VeiculoCondicao:     os.Veiculo.Condicao,
		TrajetoOrigem:       os.Trajeto.Origem,
		TrajetoReferencia:   os.Trajeto.Referencia,
		TrajetoDestino:      os.Trajeto.Destino,
		MotoristaID:         os.Motorista.ID,
		MotoristaNome:       os.Motorista.Nome,
		Guincho:             os.Guincho,
		RecebidoPor:         os.RecebidoPor,
		Observacoes:         os.Observacoes,
	}
	if !os.AcionadoEm.IsZero() {
		f.AcionadoEm = os.AcionadoEm.Local().Format("2006-01-02T15:04")
	}
	if os.Trajeto.Km > 0 {
		f.TrajetoKm = strconv.Itoa(os.Trajeto.Km)
	}
	if os.DescontoCentavos > 0 {
		f.Desconto = ordemservico.FormatarValor(os.DescontoCentavos)
	}
	for _, it := range os.Itens {
		f.Itens = append(f.Itens, formItem{
			ServicoID:     it.ServicoID,
			Descricao:     it.Descricao,
			Unidade:       it.Unidade,
			Quantidade:    ordemservico.FormatarQuantidade(it.QuantidadeCentesimos),
			ValorUnitario: ordemservico.FormatarValor(it.ValorUnitarioCentavos),
		})
	}
	return f
}

// formDaRequisicao lê os campos enviados pelo navegador.
// Os itens vêm como campos repetidos (item_descricao, item_quantidade, ...),
// alinhados pela posição.
func formDaRequisicao(r *http.Request) formOS {
	// Bytes que não são UTF-8 válido (colagem de outro sistema, terminal antigo)
	// são descartados: o Postgres recusaria a gravação inteira por causa deles.
	campo := func(nome string) string { return limpar(r.PostForm.Get(nome)) }
	f := formOS{
		DataEmissao:          campo("data_emissao"),
		AcionadoEm:           campo("acionado_em"),
		ClienteNome:          campo("cliente_nome"),
		ClienteDocumento:     campo("cliente_documento"),
		ClienteTelefone:      campo("cliente_telefone"),
		NovoClienteNome:      campo("novo_cliente_nome"),
		NovoClienteTelefone:  campo("novo_cliente_telefone"),
		NovoClienteDocumento: campo("novo_cliente_documento"),
		NovoClienteTipo:      campo("novo_cliente_tipo"),
		SolicitanteNome:      campo("solicitante_nome"),
		SolicitanteTelefone:  campo("solicitante_telefone"),
		VeiculoCategoria:     campo("veiculo_categoria"),
		VeiculoModelo:        campo("veiculo_modelo"),
		VeiculoAno:           campo("veiculo_ano"),
		VeiculoCor:           campo("veiculo_cor"),
		VeiculoPlaca:         strings.ToUpper(campo("veiculo_placa")),
		VeiculoCondicao:      campo("veiculo_condicao"),
		TrajetoOrigem:        campo("trajeto_origem"),
		TrajetoReferencia:    campo("trajeto_referencia"),
		TrajetoDestino:       campo("trajeto_destino"),
		TrajetoKm:            campo("trajeto_km"),
		MotoristaNome:        campo("motorista_nome"),
		Guincho:              campo("guincho"),
		RecebidoPor:          campo("recebido_por"),
		Observacoes:          campo("observacoes"),
		Desconto:             campo("desconto"),
	}
	f.Versao, _ = strconv.Atoi(campo("versao"))
	f.ClienteID, _ = strconv.ParseInt(campo("cliente_id"), 10, 64)
	f.MotoristaID, _ = strconv.ParseInt(campo("motorista_id"), 10, 64)
	if f.NovoClienteTipo == "" {
		f.NovoClienteTipo = "PF"
	}

	descricoes := r.PostForm["item_descricao"]
	quantidades := r.PostForm["item_quantidade"]
	valores := r.PostForm["item_valor"]
	servicos := r.PostForm["item_servico_id"]
	unidades := r.PostForm["item_unidade"]
	doKm := r.PostForm["item_do_km"]
	for i := range descricoes {
		it := formItem{Descricao: limpar(descricoes[i])}
		if i < len(quantidades) {
			it.Quantidade = limpar(quantidades[i])
		}
		if i < len(valores) {
			it.ValorUnitario = limpar(valores[i])
		}
		if i < len(servicos) {
			it.ServicoID, _ = strconv.ParseInt(servicos[i], 10, 64)
		}
		if i < len(unidades) {
			it.Unidade = limpar(unidades[i])
		}
		if i < len(doKm) {
			it.DoKm = doKm[i] == "1"
		}
		f.Itens = append(f.Itens, it)
	}
	return f
}

// aplicar converte o formulário e grava os campos editáveis na OS.
// Erros de conversão vão para f.Erros; devolve false se houver algum.
// O cliente (vínculo + cópia) é resolvido pelo handler antes desta chamada.
func (f *formOS) aplicar(os *ordemservico.OS) bool {
	f.Erros = nil

	if f.DataEmissao == "" {
		f.Erros = append(f.Erros, "Informe a data de emissão.")
	} else if data, err := time.Parse("2006-01-02", f.DataEmissao); err != nil {
		f.Erros = append(f.Erros, "Data de emissão inválida.")
	} else {
		os.DataEmissao = data
	}
	os.AcionadoEm = time.Time{}
	if f.AcionadoEm != "" {
		t, err := time.ParseInLocation("2006-01-02T15:04", f.AcionadoEm, time.Local)
		if err != nil {
			f.Erros = append(f.Erros, "Data/hora do acionamento inválida.")
		} else {
			os.AcionadoEm = t
		}
	}

	os.Solicitante = ordemservico.Solicitante{Nome: f.SolicitanteNome, Telefone: f.SolicitanteTelefone}
	os.Veiculo = ordemservico.Veiculo{
		Categoria: ordemservico.CategoriaVeiculo(f.VeiculoCategoria),
		Modelo:    f.VeiculoModelo, Ano: f.VeiculoAno, Cor: f.VeiculoCor, Placa: f.VeiculoPlaca, Condicao: f.VeiculoCondicao,
	}
	os.Trajeto = ordemservico.Trajeto{Origem: f.TrajetoOrigem, Referencia: f.TrajetoReferencia, Destino: f.TrajetoDestino}
	if f.TrajetoKm != "" {
		km, err := strconv.Atoi(f.TrajetoKm)
		if err != nil || km < 0 {
			f.Erros = append(f.Erros, "Km deve ser um número inteiro.")
		} else {
			os.Trajeto.Km = km
		}
	}
	os.Guincho = f.Guincho
	os.RecebidoPor = f.RecebidoPor
	os.Observacoes = f.Observacoes

	os.Itens = nil
	for i, it := range f.Itens {
		if it.Descricao == "" && it.Quantidade == "" && it.ValorUnitario == "" {
			continue // linha em branco
		}
		item := ordemservico.Item{ServicoID: it.ServicoID, Descricao: it.Descricao, Unidade: it.Unidade}
		var err error
		if item.QuantidadeCentesimos, err = ordemservico.ParseQuantidade(it.Quantidade); err != nil {
			f.Erros = append(f.Erros, fmt.Sprintf("Item %d: quantidade inválida.", i+1))
		}
		if item.ValorUnitarioCentavos, err = ordemservico.ParseCentavos(it.ValorUnitario); err != nil {
			f.Erros = append(f.Erros, fmt.Sprintf("Item %d: valor inválido.", i+1))
		}
		if item.Descricao == "" {
			f.Erros = append(f.Erros, fmt.Sprintf("Item %d: informe a descrição.", i+1))
		}
		os.Itens = append(os.Itens, item)
	}

	os.DescontoCentavos = 0
	if f.Desconto != "" {
		desconto, err := ordemservico.ParseCentavos(f.Desconto)
		if err != nil || desconto < 0 {
			f.Erros = append(f.Erros, "Desconto inválido.")
		} else {
			os.DescontoCentavos = desconto
		}
	}

	if len(f.Erros) == 0 {
		if err := os.Validar(); err != nil {
			var ev *ordemservico.ErroValidacao
			if errors.As(err, &ev) {
				f.Erros = append(f.Erros, ev.Mensagens...)
			} else {
				f.Erros = append(f.Erros, err.Error())
			}
		}
	}
	return len(f.Erros) == 0
}

// TotalPrevia calcula o total a partir do que está digitado, ignorando linhas
// inválidas: serve só para mostrar no formulário re-exibido.
func (f formOS) TotalPrevia() int64 {
	var total int64
	for _, it := range f.Itens {
		q, err1 := ordemservico.ParseQuantidade(it.Quantidade)
		v, err2 := ordemservico.ParseCentavos(it.ValorUnitario)
		if err1 == nil && err2 == nil {
			total += ordemservico.Item{QuantidadeCentesimos: q, ValorUnitarioCentavos: v}.Subtotal()
		}
	}
	if d, err := ordemservico.ParseCentavos(f.Desconto); err == nil {
		total -= d
	}
	return total
}

// limpar tira espaços das pontas e descarta bytes que não formam UTF-8 válido.
func limpar(s string) string {
	return strings.TrimSpace(strings.ToValidUTF8(s, ""))
}
