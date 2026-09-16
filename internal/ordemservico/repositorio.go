package ordemservico

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gustavo-Resende/autoSocorro/internal/documento"
)

var (
	ErrNaoEncontrada = errors.New("ordem de serviço não encontrada")

	// ErrConflitoVersao significa que alguém gravou a OS depois que ela foi
	// carregada por quem está tentando salvar agora.
	ErrConflitoVersao = errors.New("a OS foi alterada por outra pessoa")
)

// Repositorio é a única porta de entrada e saída da OS no banco.
type Repositorio struct {
	pool      *pgxpool.Pool
	empresaID int64

	// numeroInicial é o primeiro número que o sistema emite na vida.
	numeroInicial int

	// agora é injetável para os testes controlarem o ano.
	agora func() time.Time
}

func NovoRepositorio(pool *pgxpool.Pool, empresaID int64, numeroInicial int) *Repositorio {
	if numeroInicial < 1 {
		numeroInicial = 1
	}
	return &Repositorio{pool: pool, empresaID: empresaID, numeroInicial: numeroInicial, agora: time.Now}
}

// Criar grava uma OS nova e preenche ID, Numero e Versao.
//
// A numeração acontece na mesma transação do INSERT: um UPSERT em
// sequencia_documento incrementa o contador do ano e devolve o valor.
// O Postgres bloqueia a linha do contador até o commit, então dois usuários
// criando ao mesmo tempo recebem números diferentes, em ordem.
func (r *Repositorio) Criar(ctx context.Context, os *OS) error {
	if os.OrigemAtendimento == "" {
		os.OrigemAtendimento = OrigemParticular
	}
	os.RemoverItensVazios()
	if err := os.Validar(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ano := r.agora().Year()
	sequencial, err := r.proximoNumero(ctx, tx, documento.OS, ano)
	if err != nil {
		return err
	}
	os.Numero = Numero{Sequencial: sequencial, Ano: ano}
	os.Versao = 1

	err = tx.QueryRow(ctx, `
		INSERT INTO ordem_servico (
			empresa_id, numero, ano, versao, origem_atendimento, status, status_pagamento,
			data_emissao, valido_ate, agendada_para, acionado_em, aprovada_em, concluida_em,
			cliente_id, cliente_nome, cliente_documento, cliente_telefone,
			solicitante_nome, solicitante_telefone,
			veiculo_categoria, veiculo_modelo, veiculo_ano, veiculo_cor, veiculo_placa, veiculo_condicao,
			trajeto_origem, trajeto_referencia, trajeto_destino, trajeto_km,
			motorista_id, motorista_nome, guincho, recebido_por, observacoes,
			subtotal_centavos, desconto_centavos, total_centavos,
			valor_pago_centavos, forma_pagamento, vencimento
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19,
			$20, $21, $22, $23, $24, $25,
			$26, $27, $28, $29,
			$30, $31, $32, $33, $34,
			$35, $36, $37,
			$38, $39, $40
		)
		RETURNING id, criada_em, atualizada_em`,
		r.empresaID, os.Numero.Sequencial, os.Numero.Ano, os.Versao, os.OrigemAtendimento, os.Status, os.Pagamento.Status,
		os.DataEmissao, dataOuNulo(os.ValidoAte), tempoOuNulo(os.AgendadaPara), tempoOuNulo(os.AcionadoEm), tempoOuNulo(os.AprovadaEm), tempoOuNulo(os.ConcluidaEm),
		os.Cliente.ID, os.Cliente.Nome, os.Cliente.Documento, os.Cliente.Telefone,
		os.Solicitante.Nome, os.Solicitante.Telefone,
		os.Veiculo.Categoria, os.Veiculo.Modelo, os.Veiculo.Ano, os.Veiculo.Cor, os.Veiculo.Placa, os.Veiculo.Condicao,
		os.Trajeto.Origem, os.Trajeto.Referencia, os.Trajeto.Destino, os.Trajeto.Km,
		idOuNulo(os.Motorista.ID), os.Motorista.Nome, os.Guincho, os.RecebidoPor, os.Observacoes,
		os.Subtotal(), os.DescontoCentavos, os.Total(),
		os.Pagamento.ValorCentavos, os.Pagamento.Forma, dataOuNulo(os.Pagamento.Vencimento),
	).Scan(&os.ID, &os.CriadaEm, &os.AtualizadaEm)
	if err != nil {
		return fmt.Errorf("inserir OS: %w", err)
	}

	if err := gravarItens(ctx, tx, os.ID, os.Itens); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// proximoNumero incrementa (ou cria) o contador de empresa+tipo+ano e devolve o novo valor.
//
// Se ainda não existe nenhuma sequência deste tipo, o sistema está começando
// a vida e o contador nasce em numeroInicial (para continuar do papel).
// Anos seguintes nascem em 1. Se duas transações tentarem criar a mesma linha
// ao mesmo tempo, o ON CONFLICT transforma a segunda em UPDATE — sem duplicar.
func (r *Repositorio) proximoNumero(ctx context.Context, tx pgx.Tx, tipo documento.Tipo, ano int) (int, error) {
	var existeAlgumaSequencia bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM sequencia_documento WHERE empresa_id = $1 AND tipo = $2)`, r.empresaID, tipo,
	).Scan(&existeAlgumaSequencia)
	if err != nil {
		return 0, fmt.Errorf("consultar sequência: %w", err)
	}
	inicial := 1
	if !existeAlgumaSequencia {
		inicial = r.numeroInicial
	}

	var numero int
	err = tx.QueryRow(ctx, `
		INSERT INTO sequencia_documento (empresa_id, tipo, ano, ultimo_numero)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (empresa_id, tipo, ano) DO UPDATE
			SET ultimo_numero = sequencia_documento.ultimo_numero + 1
		RETURNING ultimo_numero`,
		r.empresaID, tipo, ano, inicial,
	).Scan(&numero)
	if err != nil {
		return 0, fmt.Errorf("gerar número: %w", err)
	}
	return numero, nil
}

// Atualizar grava todos os campos da OS, exigindo que a versão no banco seja
// a mesma de os.Versao. Em caso de sucesso, os.Versao é incrementada.
// Os itens são substituídos por inteiro (nada referencia um item).
func (r *Repositorio) Atualizar(ctx context.Context, os *OS) error {
	os.RemoverItensVazios()
	if err := os.Validar(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	novaVersao := os.Versao + 1
	tag, err := tx.Exec(ctx, `
		UPDATE ordem_servico SET
			versao = $3, status = $4, status_pagamento = $5,
			data_emissao = $6, valido_ate = $7, agendada_para = $8, acionado_em = $9, aprovada_em = $10, concluida_em = $11,
			cliente_id = $12, cliente_nome = $13, cliente_documento = $14, cliente_telefone = $15,
			solicitante_nome = $16, solicitante_telefone = $17,
			veiculo_categoria = $18, veiculo_modelo = $19, veiculo_ano = $20, veiculo_cor = $21, veiculo_placa = $22, veiculo_condicao = $23,
			trajeto_origem = $24, trajeto_referencia = $25, trajeto_destino = $26, trajeto_km = $27,
			motorista_id = $28, motorista_nome = $29, guincho = $30, recebido_por = $31, observacoes = $32,
			subtotal_centavos = $33, desconto_centavos = $34, total_centavos = $35,
			valor_pago_centavos = $36, forma_pagamento = $37, vencimento = $38,
			atualizada_em = now()
		WHERE id = $1 AND versao = $2`,
		os.ID, os.Versao, novaVersao, os.Status, os.Pagamento.Status,
		os.DataEmissao, dataOuNulo(os.ValidoAte), tempoOuNulo(os.AgendadaPara), tempoOuNulo(os.AcionadoEm), tempoOuNulo(os.AprovadaEm), tempoOuNulo(os.ConcluidaEm),
		os.Cliente.ID, os.Cliente.Nome, os.Cliente.Documento, os.Cliente.Telefone,
		os.Solicitante.Nome, os.Solicitante.Telefone,
		os.Veiculo.Categoria, os.Veiculo.Modelo, os.Veiculo.Ano, os.Veiculo.Cor, os.Veiculo.Placa, os.Veiculo.Condicao,
		os.Trajeto.Origem, os.Trajeto.Referencia, os.Trajeto.Destino, os.Trajeto.Km,
		idOuNulo(os.Motorista.ID), os.Motorista.Nome, os.Guincho, os.RecebidoPor, os.Observacoes,
		os.Subtotal(), os.DescontoCentavos, os.Total(),
		os.Pagamento.ValorCentavos, os.Pagamento.Forma, dataOuNulo(os.Pagamento.Vencimento),
	)
	if err != nil {
		return fmt.Errorf("atualizar OS: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Ou a OS não existe, ou a versão mudou. Distinguir ajuda a mensagem.
		var existe bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM ordem_servico WHERE id = $1)`, os.ID).Scan(&existe); err != nil {
			return err
		}
		if !existe {
			return ErrNaoEncontrada
		}
		return ErrConflitoVersao
	}

	if _, err := tx.Exec(ctx, `DELETE FROM ordem_servico_item WHERE ordem_servico_id = $1`, os.ID); err != nil {
		return fmt.Errorf("apagar itens: %w", err)
	}
	if err := gravarItens(ctx, tx, os.ID, os.Itens); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	os.Versao = novaVersao
	return nil
}

func gravarItens(ctx context.Context, tx pgx.Tx, osID int64, itens []Item) error {
	for i, it := range itens {
		if it.Unidade == "" {
			it.Unidade = "UN"
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO ordem_servico_item
				(ordem_servico_id, posicao, servico_id, descricao, unidade, quantidade_centesimos, valor_unitario_centavos, subtotal_centavos)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			osID, i+1, idOuNulo(it.ServicoID), it.Descricao, it.Unidade, it.QuantidadeCentesimos, it.ValorUnitarioCentavos, it.Subtotal())
		if err != nil {
			return fmt.Errorf("inserir item %d: %w", i+1, err)
		}
	}
	return nil
}

const colunasOS = `
	id, numero, ano, versao, origem_atendimento, status, status_pagamento,
	data_emissao, valido_ate, agendada_para, acionado_em, aprovada_em, concluida_em,
	cliente_id, cliente_nome, cliente_documento, cliente_telefone,
	solicitante_nome, solicitante_telefone,
	veiculo_categoria, veiculo_modelo, veiculo_ano, veiculo_cor, veiculo_placa, veiculo_condicao,
	trajeto_origem, trajeto_referencia, trajeto_destino, trajeto_km,
	motorista_id, motorista_nome, guincho, recebido_por, observacoes,
	desconto_centavos, valor_pago_centavos, forma_pagamento, vencimento,
	criada_em, atualizada_em`

// lerOS lê uma linha no formato de colunasOS.
func lerOS(linha pgx.Row) (*OS, error) {
	var os OS
	var validoAte, agendadaPara, acionadoEm, aprovadaEm, concluidaEm, vencimento *time.Time
	var motoristaID *int64
	err := linha.Scan(
		&os.ID, &os.Numero.Sequencial, &os.Numero.Ano, &os.Versao, &os.OrigemAtendimento, &os.Status, &os.Pagamento.Status,
		&os.DataEmissao, &validoAte, &agendadaPara, &acionadoEm, &aprovadaEm, &concluidaEm,
		&os.Cliente.ID, &os.Cliente.Nome, &os.Cliente.Documento, &os.Cliente.Telefone,
		&os.Solicitante.Nome, &os.Solicitante.Telefone,
		&os.Veiculo.Categoria, &os.Veiculo.Modelo, &os.Veiculo.Ano, &os.Veiculo.Cor, &os.Veiculo.Placa, &os.Veiculo.Condicao,
		&os.Trajeto.Origem, &os.Trajeto.Referencia, &os.Trajeto.Destino, &os.Trajeto.Km,
		&motoristaID, &os.Motorista.Nome, &os.Guincho, &os.RecebidoPor, &os.Observacoes,
		&os.DescontoCentavos, &os.Pagamento.ValorCentavos, &os.Pagamento.Forma, &vencimento,
		&os.CriadaEm, &os.AtualizadaEm,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrada
	}
	if err != nil {
		return nil, err
	}
	os.ValidoAte = tempoOuZero(validoAte)
	os.AgendadaPara = tempoOuZero(agendadaPara)
	os.AcionadoEm = tempoOuZero(acionadoEm)
	os.AprovadaEm = tempoOuZero(aprovadaEm)
	os.ConcluidaEm = tempoOuZero(concluidaEm)
	os.Pagamento.Vencimento = tempoOuZero(vencimento)
	if motoristaID != nil {
		os.Motorista.ID = *motoristaID
	}
	return &os, nil
}

// Buscar carrega uma OS completa (com itens) pelo id.
func (r *Repositorio) Buscar(ctx context.Context, id int64) (*OS, error) {
	os, err := lerOS(r.pool.QueryRow(ctx,
		`SELECT `+colunasOS+` FROM ordem_servico WHERE id = $1 AND empresa_id = $2`, id, r.empresaID))
	if err != nil {
		return nil, err
	}

	linhas, err := r.pool.Query(ctx, `
		SELECT coalesce(servico_id, 0), descricao, unidade, quantidade_centesimos, valor_unitario_centavos
		FROM ordem_servico_item WHERE ordem_servico_id = $1 ORDER BY posicao`, id)
	if err != nil {
		return nil, fmt.Errorf("ler itens: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var it Item
		if err := linhas.Scan(&it.ServicoID, &it.Descricao, &it.Unidade, &it.QuantidadeCentesimos, &it.ValorUnitarioCentavos); err != nil {
			return nil, err
		}
		os.Itens = append(os.Itens, it)
	}
	return os, linhas.Err()
}

// ---- conversões entre "zero" no Go e NULL no banco ----

func tempoOuNulo(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// dataOuNulo é igual a tempoOuNulo; existe só para deixar claro que a coluna é date.
func dataOuNulo(t time.Time) *time.Time { return tempoOuNulo(t) }

func tempoOuZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func idOuNulo(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
