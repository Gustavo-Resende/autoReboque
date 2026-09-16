// Package storage abstrai onde os arquivos (fotos, logo) ficam guardados.
//
// O domínio só conhece esta interface. Hoje a implementação é em disco local;
// trocar por S3/R2 no futuro é escrever outra implementação e mudar uma linha
// no main.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNaoEncontrado é devolvido por Abrir quando a chave não existe.
var ErrNaoEncontrado = errors.New("arquivo não encontrado")

// Storage guarda e devolve arquivos identificados por uma chave no estilo de
// caminho, ex.: "os/12/a1b2c3.jpg". A chave é opaca para quem usa.
type Storage interface {
	// Salvar grava o conteúdo na chave, sobrescrevendo se já existir.
	Salvar(ctx context.Context, chave string, conteudo io.Reader) error

	// Abrir devolve o conteúdo para leitura. Quem chama fecha.
	Abrir(ctx context.Context, chave string) (io.ReadCloser, error)

	// Remover apaga a chave. Remover algo que não existe não é erro.
	Remover(ctx context.Context, chave string) error
}
