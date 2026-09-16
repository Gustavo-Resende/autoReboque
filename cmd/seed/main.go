// seed popula o banco com dados fictícios para testar as telas e o dashboard:
// motoristas, clientes e umas dezenas de orçamentos/OS espalhados pelos
// últimos meses, com fotos geradas na hora.
//
//	go run ./cmd/seed            (recusa se já houver OS no banco)
//	go run ./cmd/seed -forcar    (adiciona mesmo assim)
//
// Usa a mesma configuração do servidor (.env / DATABASE_URL).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math/rand/v2"
	"os"
	"time"

	"github.com/Gustavo-Resende/autoSocorro/internal/cliente"
	"github.com/Gustavo-Resende/autoSocorro/internal/config"
	"github.com/Gustavo-Resende/autoSocorro/internal/database"
	"github.com/Gustavo-Resende/autoSocorro/internal/empresa"
	"github.com/Gustavo-Resende/autoSocorro/internal/imagem"
	"github.com/Gustavo-Resende/autoSocorro/internal/motorista"
	"github.com/Gustavo-Resende/autoSocorro/internal/ordemservico"
	"github.com/Gustavo-Resende/autoSocorro/internal/servico"
	"github.com/Gustavo-Resende/autoSocorro/internal/storage"
)

const empresaAtual int64 = 1

func main() {
	forcar := flag.Bool("forcar", false, "semear mesmo que já existam OS")
	quantidade := flag.Int("os", 70, "quantidade de orçamentos/OS a gerar")
	flag.Parse()

	if err := executar(*forcar, *quantidade); err != nil {
		slog.Error("seed falhou", "erro", err)
		os.Exit(1)
	}
}

func executar(forcar bool, quantidade int) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.Conectar(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrar(ctx, pool); err != nil {
		return err
	}

	var existentes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ordem_servico`).Scan(&existentes); err != nil {
		return err
	}
	if existentes > 0 && !forcar {
		return fmt.Errorf("já existem %d OS no banco; use -forcar para adicionar mesmo assim", existentes)
	}

	arquivos, err := storage.NovoLocal(cfg.StorageDir)
	if err != nil {
		return err
	}
	s := semeador{
		ctx:        ctx,
		rnd:        rand.New(rand.NewPCG(42, 7)), // fixo: o mesmo resultado toda vez
		ordens:     ordemservico.NovoRepositorio(pool, empresaAtual, cfg.OSNumeroInicial),
		clientes:   cliente.NovoRepositorio(pool, empresaAtual),
		servicos:   servico.NovoRepositorio(pool, empresaAtual),
		motoristas: motorista.NovoRepositorio(pool, empresaAtual),
		imagens:    imagem.NovoServico(pool, arquivos, cfg.UploadMaxBytes),
		empresa:    empresa.NovoRepositorio(pool, empresaAtual, arquivos),
	}
	return s.semear(quantidade)
}

type semeador struct {
	ctx        context.Context
	rnd        *rand.Rand
	ordens     *ordemservico.Repositorio
	clientes   *cliente.Repositorio
	servicos   *servico.Repositorio
	motoristas *motorista.Repositorio
	imagens    *imagem.Servico
	empresa    *empresa.Repositorio
}

func (s *semeador) semear(quantidade int) error {
	emp, err := s.empresa.Buscar(s.ctx)
	if err != nil {
		return err
	}

	// Motoristas.
	var motoristas []motorista.Motorista
	for _, nome := range []string{"Carlos Alberto", "Jailson Souza", "Marcos Vinícius", "Edvaldo Santos"} {
		m := &motorista.Motorista{Nome: nome, Telefone: s.telefone()}
		if err := s.motoristas.Criar(s.ctx, m); err != nil {
			return err
		}
		motoristas = append(motoristas, *m)
	}

	// Clientes: gente na estrada e empresas da região.
	var clientes []cliente.Cliente
	for _, d := range clientesFicticios {
		c := cliente.Novo()
		c.Nome, c.Tipo, c.Categoria, c.Cidade = d.nome, d.tipo, d.categoria, d.cidade
		c.Telefone = s.telefone()
		c.Documento = d.documento
		if err := s.clientes.Criar(s.ctx, c); err != nil {
			return fmt.Errorf("cliente %s: %w", d.nome, err)
		}
		clientes = append(clientes, *c)
	}

	catalogo, err := s.servicos.Listar(s.ctx, true)
	if err != nil {
		return err
	}
	porNome := map[string]servico.Servico{}
	for _, sv := range catalogo {
		porNome[sv.Nome] = sv
	}

	// OS espalhadas pelos últimos 120 dias, em ordem cronológica (a numeração
	// acompanha a data). As mais recentes têm mais chance de estar em aberto.
	hoje := time.Now().Truncate(24 * time.Hour)
	inicio := hoje.AddDate(0, 0, -120)
	datas := make([]time.Time, quantidade)
	for i := range datas {
		datas[i] = inicio.AddDate(0, 0, s.rnd.IntN(121))
	}
	ordenar(datas)

	criadas := 0
	for i, data := range datas {
		os := ordemservico.Nova(emp.OrcamentoValidadeDias)
		os.DataEmissao = data
		os.ValidoAte = data.AddDate(0, 0, emp.OrcamentoValidadeDias)

		// 1 em 8 é "serviço particular" sem cadastro.
		if s.rnd.IntN(8) == 0 {
			os.Cliente = ordemservico.Cliente{ID: cliente.ParticularID, Nome: "Serviço particular"}
		} else {
			c := clientes[s.rnd.IntN(len(clientes))]
			os.Cliente = ordemservico.Cliente{ID: c.ID, Nome: c.Nome, Documento: c.Documento, Telefone: c.Telefone}
		}
		v := veiculos[s.rnd.IntN(len(veiculos))]
		os.Veiculo = ordemservico.Veiculo{Categoria: v.categoria, Modelo: v.modelo, Ano: v.ano, Cor: cores[s.rnd.IntN(len(cores))], Placa: s.placa(), Condicao: condicoes[s.rnd.IntN(len(condicoes))]}
		km := 8 + s.rnd.IntN(140)
		os.Trajeto = ordemservico.Trajeto{Origem: origens[s.rnd.IntN(len(origens))], Destino: destinos[s.rnd.IntN(len(destinos))], Km: km}
		if s.rnd.IntN(3) == 0 {
			os.Trajeto.Referencia = referencias[s.rnd.IntN(len(referencias))]
		}
		os.AcionadoEm = data.Add(time.Duration(6+s.rnd.IntN(16)) * time.Hour)
		m := motoristas[s.rnd.IntN(len(motoristas))]
		os.Motorista = ordemservico.Motorista{ID: m.ID, Nome: m.Nome}
		os.Guincho = []string{"Plataforma 1", "Plataforma 2", "Asa-delta"}[s.rnd.IntN(3)]

		os.Itens = s.itens(porNome, km, v.categoria)
		if s.rnd.IntN(6) == 0 {
			os.DescontoCentavos = int64(10+s.rnd.IntN(8)) * 500 // 50 a 85 reais
		}
		if s.rnd.IntN(3) == 0 {
			os.Observacoes = observacoes[s.rnd.IntN(len(observacoes))]
		}

		if err := s.ordens.Criar(s.ctx, os); err != nil {
			return fmt.Errorf("criar OS %d: %w", i+1, err)
		}

		// Destino da OS conforme a idade: as antigas estão resolvidas.
		diasAtras := int(hoje.Sub(data).Hours() / 24)
		if err := s.evoluir(os, diasAtras, emp.OrcamentoValidadeDias); err != nil {
			return fmt.Errorf("evoluir OS %s: %w", os.Numero, err)
		}
		if err := s.ordens.Atualizar(s.ctx, os); err != nil {
			return fmt.Errorf("atualizar OS %s: %w", os.Numero, err)
		}

		// Fotos em algumas OS de serviço.
		if !os.EhOrcamento() && os.Status != ordemservico.Cancelada && s.rnd.IntN(4) == 0 {
			for j := range 2 + s.rnd.IntN(3) {
				etapa := imagem.Retirada
				if j%2 == 1 {
					etapa = imagem.Entrega
				}
				if _, err := s.imagens.Anexar(s.ctx, os.ID, etapa, fmt.Sprintf("foto-%d.jpg", j+1), bytes.NewReader(s.fotoFicticia())); err != nil {
					return fmt.Errorf("foto da OS %s: %w", os.Numero, err)
				}
			}
		}
		criadas++
	}

	slog.Info("semente aplicada", "motoristas", len(motoristas), "clientes", len(clientes), "os", criadas)
	return nil
}

// evoluir leva a OS recém-criada até um status plausível para a idade dela.
func (s *semeador) evoluir(os *ordemservico.OS, diasAtras, validade int) error {
	sorteio := s.rnd.IntN(100)
	mudar := func(st ordemservico.Status) error { return os.MudarStatus(st, validade) }

	switch {
	case diasAtras <= 3 && sorteio < 45:
		// Recente: costuma estar em orçamento ou em andamento.
		if sorteio < 20 {
			return nil // aberto
		}
		if sorteio < 30 {
			return mudar(ordemservico.OrcamentoEnviado)
		}
		if sorteio < 38 {
			if err := mudar(ordemservico.EmAndamento); err != nil {
				return err
			}
			return nil
		}
		os.AgendadaPara = time.Now().Add(time.Duration(6+s.rnd.IntN(60)) * time.Hour)
		return mudar(ordemservico.Agendada)
	case sorteio < 8:
		return mudar(ordemservico.OrcamentoRecusado)
	case sorteio < 12:
		// Orçamento que ficou sem resposta e expirou (a validade já passou,
		// então a transição normal recusaria; é dado fictício, gravamos direto).
		if sorteio%2 == 0 {
			os.Status = ordemservico.OrcamentoEnviado
		}
		return nil
	case sorteio < 18:
		if err := mudar(ordemservico.EmAndamento); err != nil {
			return err
		}
		os.AprovadaEm = os.DataEmissao.Add(8 * time.Hour)
		return mudar(ordemservico.Cancelada)
	default:
		if err := mudar(ordemservico.EmAndamento); err != nil {
			return err
		}
		if err := mudar(ordemservico.Concluida); err != nil {
			return err
		}
		os.AprovadaEm = os.DataEmissao.Add(time.Duration(7+s.rnd.IntN(3)) * time.Hour)
		os.ConcluidaEm = os.DataEmissao.Add(time.Duration(10+s.rnd.IntN(10)) * time.Hour)
		os.RecebidoPor = recebedores[s.rnd.IntN(len(recebedores))]
		return s.pagar(os, diasAtras)
	}
}

// pagar registra um pagamento coerente com a idade: as antigas estão quitadas.
func (s *semeador) pagar(os *ordemservico.OS, diasAtras int) error {
	total := os.Total()
	forma := formas[s.rnd.IntN(len(formas))]
	p := ordemservico.Pagamento{Forma: forma}
	switch {
	case forma == ordemservico.Faturado:
		p.Vencimento = os.DataEmissao.AddDate(0, 0, 30)
		if diasAtras > 35 || s.rnd.IntN(3) == 0 {
			p.Status, p.ValorCentavos = ordemservico.Paga, total
		} else {
			p.Status, p.ValorCentavos = ordemservico.Pendente, 0
		}
	case diasAtras > 15 || s.rnd.IntN(10) < 7:
		p.Status, p.ValorCentavos = ordemservico.Paga, total
	case s.rnd.IntN(2) == 0:
		p.Status, p.ValorCentavos = ordemservico.Parcial, (total/2/100)*100
	default:
		p.Status, p.ValorCentavos = ordemservico.Pendente, 0
	}
	return os.RegistrarPagamento(p)
}

// itens monta a lista de serviços a partir do catálogo, com preço por porte.
func (s *semeador) itens(porNome map[string]servico.Servico, km int, categoria ordemservico.CategoriaVeiculo) []ordemservico.Item {
	fator := int64(100)
	switch categoria {
	case "EXTRAPESADO":
		fator = 220
	case "UTILITARIO":
		fator = 130
	}
	item := func(nome string, qtdCentesimos int64) ordemservico.Item {
		sv := porNome[nome]
		return ordemservico.Item{ServicoID: sv.ID, Descricao: sv.Nome, Unidade: string(sv.Unidade),
			QuantidadeCentesimos: qtdCentesimos, ValorUnitarioCentavos: sv.PrecoCentavos * fator / 100}
	}
	lista := []ordemservico.Item{item("Saída de base", 100), item("Quilometragem rodada", int64(km)*100)}
	if s.rnd.IntN(3) == 0 {
		lista = append(lista, item("Hora parada / espera", int64(50+s.rnd.IntN(4)*50)))
	}
	if s.rnd.IntN(4) == 0 {
		lista = append(lista, item("Hora trabalhada", 100))
	}
	if s.rnd.IntN(3) == 0 {
		ped := item("Pedágio", int64(100+s.rnd.IntN(2)*100))
		ped.ValorUnitarioCentavos = int64(890 + s.rnd.IntN(5)*100)
		lista = append(lista, ped)
	}
	if s.rnd.IntN(5) == 0 {
		lista = append(lista, item("Adicional noturno", 100))
	}
	return lista
}

// fotoFicticia gera um JPEG 800x600 com um degradê aleatório: serve para as
// miniaturas e a impressão terem o que mostrar.
func (s *semeador) fotoFicticia() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	r0, g0, b0 := uint8(60+s.rnd.IntN(120)), uint8(60+s.rnd.IntN(120)), uint8(60+s.rnd.IntN(120))
	for y := range 600 {
		for x := range 800 {
			img.Set(x, y, color.RGBA{r0 + uint8(x/8), g0 + uint8(y/6), b0, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}

func (s *semeador) telefone() string {
	return fmt.Sprintf("(73) 9%04d-%04d", 8000+s.rnd.IntN(1999), s.rnd.IntN(10000))
}

func (s *semeador) placa() string {
	letras := "ABCDEFGHJKLMNPQRSTUVWXYZ"
	l := func() byte { return letras[s.rnd.IntN(len(letras))] }
	return fmt.Sprintf("%c%c%c%d%c%d%d", l(), l(), l(), s.rnd.IntN(10), l(), s.rnd.IntN(10), s.rnd.IntN(10))
}

func ordenar(datas []time.Time) {
	for i := 1; i < len(datas); i++ {
		for j := i; j > 0 && datas[j].Before(datas[j-1]); j-- {
			datas[j], datas[j-1] = datas[j-1], datas[j]
		}
	}
}

// ---- dados fictícios ----

var clientesFicticios = []struct {
	nome, documento string
	tipo            cliente.Tipo
	categoria       cliente.Categoria
	cidade          string
}{
	{"Maria Aparecida dos Santos", "123.456.789-00", cliente.PessoaFisica, "PARTICULAR", "Itabuna"},
	{"João Pedro Almeida", "987.654.321-11", cliente.PessoaFisica, "PARTICULAR", "Ilhéus"},
	{"Ana Beatriz Rocha", "", cliente.PessoaFisica, "PARTICULAR", "Itabuna"},
	{"Roberto Carlos Lima", "456.789.123-22", cliente.PessoaFisica, "PARTICULAR", "Buerarema"},
	{"Fernanda Oliveira", "", cliente.PessoaFisica, "PARTICULAR", "Uruçuca"},
	{"Luiz Henrique Souza", "321.654.987-33", cliente.PessoaFisica, "PARTICULAR", "Itabuna"},
	{"Oficina do Zé Mecânica", "12.345.678/0001-90", cliente.PessoaJuridica, "OFICINA", "Itabuna"},
	{"Auto Center Cacau", "23.456.789/0001-01", cliente.PessoaJuridica, "OFICINA", "Ilhéus"},
	{"Localiza Rent a Car - Ilhéus", "34.567.890/0001-12", cliente.PessoaJuridica, "LOCADORA", "Ilhéus"},
	{"Movida Aluguel de Carros", "45.678.901/0001-23", cliente.PessoaJuridica, "LOCADORA", "Itabuna"},
	{"Concessionária Sul Bahia Fiat", "56.789.012/0001-34", cliente.PessoaJuridica, "CONCESSIONARIA", "Itabuna"},
	{"Prefeitura Municipal de Itabuna", "67.890.123/0001-45", cliente.PessoaJuridica, "ORGAO_PUBLICO", "Itabuna"},
	{"Transportadora Grapiúna", "78.901.234/0001-56", cliente.PessoaJuridica, "OUTRO", "Itabuna"},
	{"Cooperativa de Táxi Ilhéus", "89.012.345/0001-67", cliente.PessoaJuridica, "OUTRO", "Ilhéus"},
}

var veiculos = []struct {
	modelo, ano string
	categoria   ordemservico.CategoriaVeiculo
}{
	{"Fiat Strada 1.4", "2019/2020", "UTILITARIO"}, {"Chevrolet Onix 1.0", "2021/2022", "LEVE"},
	{"Volkswagen Gol 1.6", "2015/2016", "LEVE"}, {"Honda CG 160 Titan", "2020", "LEVE"},
	{"Toyota Corolla XEi", "2018/2019", "LEVE"}, {"Fiat Toro Freedom", "2022", "UTILITARIO"},
	{"Hyundai HB20", "2017/2018", "LEVE"}, {"Renault Master Furgão", "2016", "UTILITARIO"},
	{"Mercedes-Benz Accelo 815", "2014", "EXTRAPESADO"}, {"Yamaha Fazer 250", "2019", "LEVE"},
	{"Jeep Renegade", "2020/2021", "LEVE"}, {"Fiat Uno Mille", "2010", "LEVE"},
	{"Volkswagen Saveiro", "2013", "UTILITARIO"}, {"Trator Massey Ferguson 275", "2008", "EXTRAPESADO"},
}

var cores = []string{"Branco", "Prata", "Preto", "Vermelho", "Cinza", "Azul", "Verde"}
var condicoes = []string{"Roda", "Travado", "Sem chave", "Batido", "Capotado", "Roda", "Roda"}
var origens = []string{"BR-101, km 512", "BR-415, km 30, Itabuna", "Av. Cinquentenário, Itabuna", "BA-262, próximo a Uruçuca", "Rod. Ilhéus–Olivença, km 7", "Centro de Buerarema", "Pontal, Ilhéus", "BR-101, km 480, trevo de Itapé"}
var referencias = []string{"Posto Trevo", "em frente ao Atacadão", "depois da ponte do Rio Cachoeira", "ao lado da igreja", "km 3 da estrada de terra"}
var destinos = []string{"Oficina do Zé, Itabuna", "Auto Center Cacau, Ilhéus", "Concessionária Fiat, Itabuna", "Residência do cliente, Itabuna", "Pátio da Localiza, Ilhéus", "Oficina Mecânica Central, Itabuna", "Garagem da prefeitura"}
var observacoes = []string{"Veículo com suspeita de motor fundido. Chave entregue ao responsável da oficina.", "Cliente acompanhou o transporte.", "Pneu dianteiro esquerdo furado; risco pré-existente na porta traseira.", "Retirado em local de difícil acesso, precisou de manobra com cabo.", "Cliente pediu para avisar a oficina antes da chegada."}
var recebedores = []string{"Zé (oficina)", "Cliente", "Segurança do pátio", "Recepção", "Mecânico de plantão"}
var formas = []ordemservico.FormaPagamento{ordemservico.PIX, ordemservico.PIX, ordemservico.PIX, ordemservico.Dinheiro, ordemservico.Credito, ordemservico.Debito, ordemservico.Faturado, ordemservico.Faturado, ordemservico.Transferencia}
