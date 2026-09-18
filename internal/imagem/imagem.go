// Package imagem cuida das fotos anexadas à OS: validação do upload,
// geração de miniatura e o registro no banco. Os bytes ficam no Storage.
package imagem

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gustavo-Resende/autoReboque/internal/storage"
)

var (
	ErrNaoEncontrada = errors.New("imagem não encontrada")
	ErrArquivoGrande = errors.New("arquivo maior que o limite permitido")
)

// Etapa diz em que momento a foto foi tirada. Na retirada ela documenta o
// estado do veículo (proteção contra "esse risco não estava aí"); na entrega,
// que chegou como saiu.
type Etapa string

const (
	Retirada Etapa = "RETIRADA"
	Entrega  Etapa = "ENTREGA"
	Outra    Etapa = "OUTRA"
)

var TodasEtapas = []Etapa{Retirada, Entrega, Outra}

func (e Etapa) Valida() bool {
	return e == Retirada || e == Entrega || e == Outra
}

func (e Etapa) Rotulo() string {
	switch e {
	case Retirada:
		return "Retirada"
	case Entrega:
		return "Entrega"
	}
	return "Outras"
}

// Imagem é uma foto anexada a uma OS.
type Imagem struct {
	ID             int64
	OrdemServicoID int64
	Posicao        int
	Etapa          Etapa
	ChaveOriginal  string
	ChaveMiniatura string
	NomeOriginal   string
	ContentType    string
	TamanhoBytes   int64
	CriadaEm       time.Time
}

// Servico é a porta de entrada das operações com imagens.
type Servico struct {
	pool     *pgxpool.Pool
	storage  storage.Storage
	maxBytes int64
}

func NovoServico(pool *pgxpool.Pool, st storage.Storage, maxBytes int64) *Servico {
	return &Servico{pool: pool, storage: st, maxBytes: maxBytes}
}

// LerLimitado lê um upload respeitando o limite de tamanho configurado.
func (s *Servico) LerLimitado(conteudo io.Reader) ([]byte, error) {
	dados, err := io.ReadAll(io.LimitReader(conteudo, s.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("ler arquivo: %w", err)
	}
	if int64(len(dados)) > s.maxBytes {
		return nil, ErrArquivoGrande
	}
	return dados, nil
}

// Anexar valida a imagem, gera a miniatura, guarda as duas no Storage e
// registra a linha. A ordem é: arquivos primeiro, banco por último — se o
// banco falhar, sobram arquivos órfãos (inofensivo); o contrário deixaria
// um registro apontando para o nada.
func (s *Servico) Anexar(ctx context.Context, osID int64, etapa Etapa, nomeOriginal string, conteudo io.Reader) (*Imagem, error) {
	if !etapa.Valida() {
		etapa = Outra
	}
	dados, err := s.LerLimitado(conteudo)
	if err != nil {
		return nil, err
	}
	formato, err := Detectar(dados)
	if err != nil {
		return nil, err
	}
	miniatura, err := GerarMiniatura(dados)
	if err != nil {
		return nil, fmt.Errorf("gerar miniatura: %w", err)
	}

	base := fmt.Sprintf("os/%d/%s", osID, identificadorAleatorio())
	img := &Imagem{
		OrdemServicoID: osID,
		Etapa:          etapa,
		ChaveOriginal:  base + formato.Extensao,
		ChaveMiniatura: base + "_mini.jpg",
		NomeOriginal:   nomeOriginal,
		ContentType:    formato.ContentType,
		TamanhoBytes:   int64(len(dados)),
	}
	if err := s.storage.Salvar(ctx, img.ChaveOriginal, bytes.NewReader(dados)); err != nil {
		return nil, fmt.Errorf("salvar imagem: %w", err)
	}
	if err := s.storage.Salvar(ctx, img.ChaveMiniatura, bytes.NewReader(miniatura)); err != nil {
		_ = s.storage.Remover(ctx, img.ChaveOriginal)
		return nil, fmt.Errorf("salvar miniatura: %w", err)
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO ordem_servico_imagem
			(ordem_servico_id, posicao, etapa, chave_original, chave_miniatura, nome_original, content_type, tamanho_bytes)
		VALUES ($1,
			(SELECT coalesce(max(posicao), 0) + 1 FROM ordem_servico_imagem WHERE ordem_servico_id = $1),
			$2, $3, $4, $5, $6, $7)
		RETURNING id, posicao, criada_em`,
		osID, img.Etapa, img.ChaveOriginal, img.ChaveMiniatura, img.NomeOriginal, img.ContentType, img.TamanhoBytes,
	).Scan(&img.ID, &img.Posicao, &img.CriadaEm)
	if err != nil {
		_ = s.storage.Remover(ctx, img.ChaveOriginal)
		_ = s.storage.Remover(ctx, img.ChaveMiniatura)
		return nil, fmt.Errorf("registrar imagem: %w", err)
	}
	return img, nil
}

const colunas = `id, ordem_servico_id, posicao, etapa, chave_original, chave_miniatura, nome_original, content_type, tamanho_bytes, criada_em`

func ler(linha pgx.Row) (*Imagem, error) {
	var img Imagem
	err := linha.Scan(&img.ID, &img.OrdemServicoID, &img.Posicao, &img.Etapa, &img.ChaveOriginal, &img.ChaveMiniatura,
		&img.NomeOriginal, &img.ContentType, &img.TamanhoBytes, &img.CriadaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrada
	}
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// ListarDaOS devolve as imagens de uma OS agrupadas por etapa
// (retirada, entrega, outras) e, dentro da etapa, na ordem em que foram anexadas.
func (s *Servico) ListarDaOS(ctx context.Context, osID int64) ([]Imagem, error) {
	linhas, err := s.pool.Query(ctx,
		`SELECT `+colunas+` FROM ordem_servico_imagem WHERE ordem_servico_id = $1
		 ORDER BY CASE etapa WHEN 'RETIRADA' THEN 1 WHEN 'ENTREGA' THEN 2 ELSE 3 END, posicao`, osID)
	if err != nil {
		return nil, fmt.Errorf("listar imagens: %w", err)
	}
	defer linhas.Close()
	var lista []Imagem
	for linhas.Next() {
		img, err := ler(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, *img)
	}
	return lista, linhas.Err()
}

func (s *Servico) Buscar(ctx context.Context, id int64) (*Imagem, error) {
	return ler(s.pool.QueryRow(ctx, `SELECT `+colunas+` FROM ordem_servico_imagem WHERE id = $1`, id))
}

// Remover apaga o registro e depois os arquivos.
func (s *Servico) Remover(ctx context.Context, id int64) error {
	img, err := s.Buscar(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM ordem_servico_imagem WHERE id = $1`, id); err != nil {
		return fmt.Errorf("apagar registro da imagem: %w", err)
	}
	_ = s.storage.Remover(ctx, img.ChaveOriginal)
	_ = s.storage.Remover(ctx, img.ChaveMiniatura)
	return nil
}

// AlterarEtapa muda a etapa de uma foto já anexada.
func (s *Servico) AlterarEtapa(ctx context.Context, id int64, etapa Etapa) error {
	if !etapa.Valida() {
		return fmt.Errorf("etapa inválida: %q", etapa)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE ordem_servico_imagem SET etapa = $2 WHERE id = $1`, id, etapa)
	if err != nil {
		return fmt.Errorf("alterar etapa: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrada
	}
	return nil
}

// Abrir devolve o conteúdo de um arquivo do Storage (original ou miniatura).
func (s *Servico) Abrir(ctx context.Context, chave string) (io.ReadCloser, error) {
	return s.storage.Abrir(ctx, chave)
}

func identificadorAleatorio() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// rand.Read só falha se o sistema operacional estiver quebrado.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
