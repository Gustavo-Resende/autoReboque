package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalSalvarAbrirRemover(t *testing.T) {
	s, err := NovoLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := s.Salvar(ctx, "os/1/foto.jpg", strings.NewReader("conteudo")); err != nil {
		t.Fatal(err)
	}
	f, err := s.Abrir(ctx, "os/1/foto.jpg")
	if err != nil {
		t.Fatal(err)
	}
	dados, _ := io.ReadAll(f)
	f.Close()
	if string(dados) != "conteudo" {
		t.Errorf("conteúdo lido = %q", dados)
	}

	if err := s.Remover(ctx, "os/1/foto.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Abrir(ctx, "os/1/foto.jpg"); !errors.Is(err, ErrNaoEncontrado) {
		t.Errorf("após remover, esperado ErrNaoEncontrado, veio %v", err)
	}
	if err := s.Remover(ctx, "os/1/foto.jpg"); err != nil {
		t.Errorf("remover inexistente não deveria dar erro: %v", err)
	}
}

func TestLocalRecusaSairDaPasta(t *testing.T) {
	s, _ := NovoLocal(t.TempDir())
	for _, chave := range []string{"", "../fora.txt", "os/../../fora.txt", "/absoluto", "a\b"} {
		if err := s.Salvar(context.Background(), chave, strings.NewReader("x")); err == nil {
			t.Errorf("chave %q deveria ser recusada", chave)
		}
	}
}
