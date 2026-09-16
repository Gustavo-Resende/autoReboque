package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Local guarda os arquivos em uma pasta do disco.
type Local struct {
	dir string
}

// NovoLocal cria (se preciso) a pasta base e devolve o storage.
func NovoLocal(dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("criar pasta de storage %s: %w", abs, err)
	}
	return &Local{dir: abs}, nil
}

// caminho converte a chave em caminho absoluto, recusando qualquer tentativa
// de sair da pasta base (ex.: "../../etc/passwd").
func (l *Local) caminho(chave string) (string, error) {
	if chave == "" || strings.HasPrefix(chave, "/") || strings.Contains(chave, "\\") {
		return "", fmt.Errorf("chave inválida: %q", chave)
	}
	limpo := filepath.Clean(filepath.FromSlash(chave))
	if limpo == "." || strings.HasPrefix(limpo, "..") {
		return "", fmt.Errorf("chave inválida: %q", chave)
	}
	completo := filepath.Join(l.dir, limpo)
	if !strings.HasPrefix(completo, l.dir+string(filepath.Separator)) {
		return "", fmt.Errorf("chave inválida: %q", chave)
	}
	return completo, nil
}

func (l *Local) Salvar(ctx context.Context, chave string, conteudo io.Reader) error {
	destino, err := l.caminho(chave)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return err
	}
	// Grava em arquivo temporário e renomeia: nunca fica um arquivo pela metade.
	tmp, err := os.CreateTemp(filepath.Dir(destino), ".tmp-*")
	if err != nil {
		return err
	}
	nomeTmp := tmp.Name()
	if _, err := io.Copy(tmp, conteudo); err != nil {
		tmp.Close()
		os.Remove(nomeTmp)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(nomeTmp)
		return err
	}
	if err := os.Rename(nomeTmp, destino); err != nil {
		os.Remove(nomeTmp)
		return err
	}
	return nil
}

func (l *Local) Abrir(ctx context.Context, chave string) (io.ReadCloser, error) {
	origem, err := l.caminho(chave)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(origem)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNaoEncontrado
	}
	return f, err
}

func (l *Local) Remover(ctx context.Context, chave string) error {
	alvo, err := l.caminho(chave)
	if err != nil {
		return err
	}
	err = os.Remove(alvo)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
