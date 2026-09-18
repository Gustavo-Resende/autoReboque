package ordemservico

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/database"
)

// Estes testes precisam de Postgres (TEST_DATABASE_URL). Sem ele, são pulados.

const empresaTeste int64 = 1

func repositorioDeTeste(t *testing.T, numeroInicial int) *Repositorio {
	t.Helper()
	return NovoRepositorio(database.PoolDeTeste(t), empresaTeste, numeroInicial)
}

func TestCriarNumeraEmSequencia(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()
	ano := time.Now().Year()

	for esperado := 1; esperado <= 3; esperado++ {
		os := novaComCliente()
		if err := repo.Criar(ctx, os); err != nil {
			t.Fatal(err)
		}
		if os.ID == 0 || os.Versao != 1 {
			t.Errorf("OS criada sem id/versão: id=%d versão=%d", os.ID, os.Versao)
		}
		if os.Numero.Sequencial != esperado || os.Numero.Ano != ano {
			t.Errorf("número = %s, esperado %04d/%d", os.Numero, esperado, ano)
		}
	}
}

func TestNumeroInicialSoValeParaAPrimeiraSequencia(t *testing.T) {
	repo := repositorioDeTeste(t, 143)
	ctx := context.Background()

	// Ano de entrada em produção: começa em 143.
	repo.agora = func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }
	os := novaComCliente()
	if err := repo.Criar(ctx, os); err != nil {
		t.Fatal(err)
	}
	if os.Numero.String() != "0143/2026" {
		t.Errorf("primeiro número = %s, esperado 0143/2026", os.Numero)
	}
	os = novaComCliente()
	if err := repo.Criar(ctx, os); err != nil {
		t.Fatal(err)
	}
	if os.Numero.String() != "0144/2026" {
		t.Errorf("segundo número = %s, esperado 0144/2026", os.Numero)
	}

	// Ano seguinte: volta para 1.
	repo.agora = func() time.Time { return time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC) }
	os = novaComCliente()
	if err := repo.Criar(ctx, os); err != nil {
		t.Fatal(err)
	}
	if os.Numero.String() != "0001/2027" {
		t.Errorf("primeiro número do ano seguinte = %s, esperado 0001/2027", os.Numero)
	}
}

// Várias goroutines criando OS ao mesmo tempo não podem receber o mesmo número
// nem pular nenhum: o resultado tem que ser exatamente 1..N.
func TestCriarConcorrenteNaoRepeteNumero(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()
	const n = 40

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		numeros []int
		erros   []error
	)
	for range n {
		wg.Go(func() {
			os := novaComCliente()
			os.Itens = []Item{{Descricao: "Saída de base", QuantidadeCentesimos: 100, ValorUnitarioCentavos: 10000}}
			err := repo.Criar(ctx, os)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				erros = append(erros, err)
				return
			}
			numeros = append(numeros, os.Numero.Sequencial)
		})
	}
	wg.Wait()

	if len(erros) > 0 {
		t.Fatalf("%d criações falharam; primeira: %v", len(erros), erros[0])
	}
	sort.Ints(numeros)
	for i, num := range numeros {
		if num != i+1 {
			t.Fatalf("sequência quebrada: %v", numeros)
		}
	}
}

func TestAtualizarComOptimisticLocking(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()

	original := novaComCliente()
	original.Veiculo.Placa = "ABC1D23"
	if err := repo.Criar(ctx, original); err != nil {
		t.Fatal(err)
	}

	// Duas pessoas carregam a mesma OS (versão 1).
	pc, err := repo.Buscar(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	celular, err := repo.Buscar(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}

	// O PC salva primeiro: passa, e a versão vai para 2.
	pc.Veiculo.Cor = "Prata"
	if err := repo.Atualizar(ctx, pc); err != nil {
		t.Fatalf("primeira gravação deveria passar: %v", err)
	}
	if pc.Versao != 2 {
		t.Errorf("versão após gravar = %d, esperado 2", pc.Versao)
	}

	// O celular ainda tem a versão 1: tem que ser recusado sem sobrescrever.
	celular.Veiculo.Placa = "XYZ9K88"
	err = repo.Atualizar(ctx, celular)
	if !errors.Is(err, ErrConflitoVersao) {
		t.Fatalf("segunda gravação deveria dar ErrConflitoVersao, veio %v", err)
	}

	noBanco, err := repo.Buscar(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if noBanco.Veiculo.Placa != "ABC1D23" || noBanco.Veiculo.Cor != "Prata" || noBanco.Versao != 2 {
		t.Errorf("banco deveria ter só a gravação do PC: %+v", noBanco.Veiculo)
	}

	// Depois de recarregar, o celular consegue gravar.
	celular, _ = repo.Buscar(ctx, original.ID)
	celular.Veiculo.Placa = "XYZ9K88"
	if err := repo.Atualizar(ctx, celular); err != nil {
		t.Fatalf("gravação após recarregar deveria passar: %v", err)
	}
}

func TestIdaEVoltaDeTodosOsCampos(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()

	os := novaComCliente()
	os.Solicitante = Solicitante{Nome: "Oficina do Zé", Telefone: "73 99999-0000"}
	os.Veiculo = Veiculo{Categoria: "LEVE", Modelo: "Onix", Ano: "2019/2020", Cor: "Prata", Placa: "XYZ9K88", Condicao: "Travado"}
	os.Trajeto = Trajeto{Origem: "BR-101 km 512", Referencia: "Posto Trevo", Destino: "Oficina", Km: 87}
	os.Motorista = Motorista{Nome: "Carlos"}
	os.Guincho = "Plataforma 1"
	os.RecebidoPor = "Zé"
	os.Observacoes = "Sem chave"
	os.AcionadoEm = time.Date(2026, 9, 15, 8, 30, 0, 0, time.UTC)
	os.Itens = []Item{
		{ServicoID: 1, Descricao: "Saída de base", Unidade: "UN", QuantidadeCentesimos: 100, ValorUnitarioCentavos: 15000},
		{ServicoID: 2, Descricao: "Km rodada", Unidade: "KM", QuantidadeCentesimos: 8700, ValorUnitarioCentavos: 450},
	}
	os.DescontoCentavos = 1500
	if err := os.RegistrarPagamento(Pagamento{Status: Parcial, ValorCentavos: 10000, Forma: Faturado, Vencimento: hoje().AddDate(0, 0, 30)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Criar(ctx, os); err != nil {
		t.Fatal(err)
	}

	lido, err := repo.Buscar(ctx, os.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lido.Solicitante != os.Solicitante || lido.Veiculo != os.Veiculo || lido.Trajeto != os.Trajeto ||
		lido.Motorista != os.Motorista || lido.Guincho != os.Guincho || lido.RecebidoPor != os.RecebidoPor ||
		lido.DescontoCentavos != os.DescontoCentavos || !lido.AcionadoEm.Equal(os.AcionadoEm) ||
		!lido.ValidoAte.Equal(os.ValidoAte) || !lido.Pagamento.Vencimento.Equal(os.Pagamento.Vencimento) ||
		lido.Pagamento.Forma != Faturado || lido.Pagamento.ValorCentavos != 10000 {
		t.Errorf("campos não voltaram iguais:\n got %+v\nwant %+v", lido, os)
	}
	if len(lido.Itens) != 2 || lido.Itens[1].ServicoID != 2 || lido.Itens[1].Unidade != "KM" {
		t.Errorf("itens não voltaram iguais: %+v", lido.Itens)
	}
	if lido.Total() != 15000+39150-1500 {
		t.Errorf("total = %d", lido.Total())
	}
}

func TestAtualizarOSInexistente(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	os := novaComCliente()
	os.ID = 999999
	os.Versao = 1
	if err := repo.Atualizar(context.Background(), os); !errors.Is(err, ErrNaoEncontrada) {
		t.Errorf("esperado ErrNaoEncontrada, veio %v", err)
	}
	if _, err := repo.Buscar(context.Background(), 999999); !errors.Is(err, ErrNaoEncontrada) {
		t.Errorf("Buscar: esperado ErrNaoEncontrada, veio %v", err)
	}
}

// criarComStatus cria uma OS e a leva até o status pedido, com o total dado.
func criarComStatus(t *testing.T, repo *Repositorio, nome string, status Status, total int64, dataEmissao time.Time) *OS {
	t.Helper()
	ctx := context.Background()
	os := novaComCliente()
	os.Cliente.Nome = nome
	os.DataEmissao = dataEmissao
	os.Itens = []Item{{Descricao: "Serviço", QuantidadeCentesimos: 100, ValorUnitarioCentavos: total}}
	if err := repo.Criar(ctx, os); err != nil {
		t.Fatal(err)
	}
	caminho := map[Status][]Status{
		OrcamentoAberto:   {},
		OrcamentoEnviado:  {OrcamentoEnviado},
		OrcamentoRecusado: {OrcamentoRecusado},
		Agendada:          {Agendada},
		EmAndamento:       {EmAndamento},
		Concluida:         {EmAndamento, Concluida},
		Cancelada:         {EmAndamento, Cancelada},
	}
	for _, passo := range caminho[status] {
		if passo == Agendada {
			os.AgendadaPara = time.Now().Add(48 * time.Hour)
		}
		if err := os.MudarStatus(passo, validadeTeste); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Atualizar(ctx, os); err != nil {
		t.Fatal(err)
	}
	return os
}

func TestListarPorFaseStatusEBusca(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()
	hoje := hoje()

	criarComStatus(t, repo, "Ana Souza", OrcamentoAberto, 10000, hoje)
	enviado := criarComStatus(t, repo, "Bruno Lima", OrcamentoEnviado, 20000, hoje)
	criarComStatus(t, repo, "Carla Dias", EmAndamento, 30000, hoje)
	criarComStatus(t, repo, "Daniel Reis", Concluida, 40000, hoje)
	criarComStatus(t, repo, "Elisa Melo", Cancelada, 50000, hoje)

	// Um orçamento enviado com validade vencida aparece como expirado.
	enviado.ValidoAte = hoje.AddDate(0, 0, -1)
	if err := repo.Atualizar(ctx, enviado); err != nil {
		t.Fatal(err)
	}

	lista, err := repo.Listar(ctx, Filtro{Fase: FaseOrcamento})
	if err != nil {
		t.Fatal(err)
	}
	if len(lista.Linhas) != 2 {
		t.Errorf("fase orçamento: %d linhas, esperado 2", len(lista.Linhas))
	}
	lista, _ = repo.Listar(ctx, Filtro{Fase: FaseOS})
	if len(lista.Linhas) != 3 || lista.Resumo.TotalCentavos != 120000 {
		t.Errorf("fase OS: %d linhas, total %d", len(lista.Linhas), lista.Resumo.TotalCentavos)
	}

	lista, _ = repo.Listar(ctx, Filtro{Status: OrcamentoExpirado})
	if len(lista.Linhas) != 1 || lista.Linhas[0].ClienteNome != "Bruno Lima" || lista.Linhas[0].Status != OrcamentoExpirado {
		t.Errorf("filtro expirado: %+v", lista.Linhas)
	}
	lista, _ = repo.Listar(ctx, Filtro{Status: OrcamentoEnviado})
	if len(lista.Linhas) != 0 {
		t.Errorf("enviado vencido não deveria contar como enviado: %+v", lista.Linhas)
	}

	lista, _ = repo.Listar(ctx, Filtro{Busca: "0004/"})
	if len(lista.Linhas) != 1 || lista.Linhas[0].ClienteNome != "Daniel Reis" {
		t.Errorf("busca por número: %+v", lista.Linhas)
	}

	contagem, err := repo.ContarPorStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if contagem[OrcamentoAberto] != 1 || contagem[OrcamentoExpirado] != 1 || contagem[EmAndamento] != 1 ||
		contagem[Concluida] != 1 || contagem[Cancelada] != 1 {
		t.Errorf("contagem por status: %v", contagem)
	}
}

// O dashboard só conta a fase de OS; orçamento é rascunho e cancelada não existe.
func TestDashboardIgnoraOrcamentoECancelada(t *testing.T) {
	repo := repositorioDeTeste(t, 1)
	ctx := context.Background()
	hoje := hoje()
	mesPassado := hoje.AddDate(0, -1, 0)

	criarComStatus(t, repo, "Orçamento aberto", OrcamentoAberto, 99900, hoje)
	criarComStatus(t, repo, "Recusado", OrcamentoRecusado, 88800, hoje)
	criarComStatus(t, repo, "Cancelada", Cancelada, 77700, hoje)
	andamento := criarComStatus(t, repo, "Em andamento", EmAndamento, 30000, hoje)
	concluida := criarComStatus(t, repo, "Concluída", Concluida, 50000, hoje)
	criarComStatus(t, repo, "Mês passado", Concluida, 20000, mesPassado)

	if err := andamento.RegistrarPagamento(Pagamento{Status: Parcial, ValorCentavos: 10000, Forma: PIX}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Atualizar(ctx, andamento); err != nil {
		t.Fatal(err)
	}
	if err := concluida.RegistrarPagamento(Pagamento{Status: Paga, ValorCentavos: 50000, Forma: Dinheiro}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Atualizar(ctx, concluida); err != nil {
		t.Fatal(err)
	}

	ind, err := repo.Dashboard(ctx, MesDe(hoje))
	if err != nil {
		t.Fatal(err)
	}
	if ind.OSQuantidade != 2 || ind.FaturadoCentavos != 80000 || ind.RecebidoCentavos != 60000 {
		t.Errorf("período: qtd=%d faturado=%d recebido=%d", ind.OSQuantidade, ind.FaturadoCentavos, ind.RecebidoCentavos)
	}
	if ind.TicketMedioCentavos != 40000 {
		t.Errorf("ticket médio = %d", ind.TicketMedioCentavos)
	}
	if ind.Aprovadas != 2 || ind.Recusadas != 1 || ind.ConversaoPercentual != 66 {
		t.Errorf("conversão: aprovadas=%d recusadas=%d %%=%d", ind.Aprovadas, ind.Recusadas, ind.ConversaoPercentual)
	}
	// A receber é global: 20.000 (em andamento) + 20.000 (mês passado, pendente).
	if ind.AReceberCentavos != 40000 || ind.AReceberQuantidade != 2 {
		t.Errorf("a receber = %d (%d OS)", ind.AReceberCentavos, ind.AReceberQuantidade)
	}
	if ind.OrcamentosAbertos != 1 || ind.OrcamentosAbertosCentavos != 99900 {
		t.Errorf("orçamentos abertos = %d (%d)", ind.OrcamentosAbertos, ind.OrcamentosAbertosCentavos)
	}
	if ind.EmAndamento != 1 || ind.ConcluidasHoje != 2 {
		t.Errorf("em andamento = %d, concluídas hoje = %d", ind.EmAndamento, ind.ConcluidasHoje)
	}
	if len(ind.FaturamentoMensal) != 6 {
		t.Fatalf("faturamento mensal deveria ter 6 meses, tem %d", len(ind.FaturamentoMensal))
	}
	ultimo := ind.FaturamentoMensal[5]
	penultimo := ind.FaturamentoMensal[4]
	if ultimo.Centavos != 80000 || penultimo.Centavos != 20000 {
		t.Errorf("faturamento mensal: %+v", ind.FaturamentoMensal)
	}
	if len(ind.PorFormaPagamento) != 2 || ind.PorFormaPagamento[0].Forma != Dinheiro {
		t.Errorf("por forma: %+v", ind.PorFormaPagamento)
	}
}
