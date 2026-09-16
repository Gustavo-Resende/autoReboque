package cliente

import (
	"context"
	"errors"
	"testing"

	"github.com/Gustavo-Resende/autoSocorro/internal/database"
)

func TestValidar(t *testing.T) {
	c := Novo()
	if err := c.Validar(); !errors.Is(err, ErrNomeObrigatorio) {
		t.Errorf("sem nome: esperado ErrNomeObrigatorio, veio %v", err)
	}
	c.Nome = "  Maria  "
	c.Documento = " 123 "
	if err := c.Validar(); err != nil {
		t.Fatal(err)
	}
	if c.Nome != "Maria" || c.Documento != "123" {
		t.Errorf("Validar deveria tirar espaços: %q %q", c.Nome, c.Documento)
	}
	c.Categoria = "INVENTADA"
	if err := c.Validar(); err == nil {
		t.Error("categoria inválida deveria ser recusada")
	}
}

func TestSomenteDigitos(t *testing.T) {
	if got := SomenteDigitos("(73) 99986-0359"); got != "73999860359" {
		t.Errorf("got %q", got)
	}
}

// Precisa de Postgres (TEST_DATABASE_URL).
func TestRepositorio(t *testing.T) {
	repo := NovoRepositorio(database.PoolDeTeste(t), 1)
	ctx := context.Background()

	// O cliente de sistema existe desde a migration e é intocável.
	particular, err := repo.Buscar(ctx, ParticularID)
	if err != nil {
		t.Fatal(err)
	}
	if !particular.Sistema || particular.Nome != "Serviço particular" {
		t.Errorf("cliente de sistema inesperado: %+v", particular)
	}
	particular.Nome = "Outro"
	if err := repo.Atualizar(ctx, particular); !errors.Is(err, ErrClienteDeSistema) {
		t.Errorf("editar cliente de sistema: esperado ErrClienteDeSistema, veio %v", err)
	}
	if err := repo.DefinirAtivo(ctx, ParticularID, false); !errors.Is(err, ErrNaoEncontrado) {
		t.Errorf("desativar cliente de sistema deveria falhar, veio %v", err)
	}

	maria := Novo()
	maria.Nome = "Maria Aparecida"
	maria.Documento = "123.456.789-00"
	maria.Telefone = "(73) 98888-1234"
	if err := repo.Criar(ctx, maria); err != nil {
		t.Fatal(err)
	}

	// Documento repetido é recusado; documento vazio pode repetir.
	clone := Novo()
	clone.Nome = "Maria Clone"
	clone.Documento = "123.456.789-00"
	if err := repo.Criar(ctx, clone); !errors.Is(err, ErrDocumentoDuplicado) {
		t.Errorf("documento duplicado: esperado ErrDocumentoDuplicado, veio %v", err)
	}
	for _, nome := range []string{"Sem doc 1", "Sem doc 2"} {
		c := Novo()
		c.Nome = nome
		if err := repo.Criar(ctx, c); err != nil {
			t.Errorf("cliente sem documento deveria ser aceito: %v", err)
		}
	}

	sugestoes, err := repo.Sugerir(ctx, "9888", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugestoes) != 1 || sugestoes[0].ID != maria.ID {
		t.Errorf("sugerir por telefone: %+v", sugestoes)
	}

	if err := repo.DefinirAtivo(ctx, maria.ID, false); err != nil {
		t.Fatal(err)
	}
	lista, err := repo.Listar(ctx, Filtro{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lista.Clientes {
		if c.ID == maria.ID {
			t.Error("cliente inativo não deveria aparecer na lista padrão")
		}
	}
	// O de sistema vai por último, mesmo em ordem alfabética.
	if lista.Clientes[len(lista.Clientes)-1].ID != ParticularID {
		t.Errorf("cliente de sistema deveria ser o último: %+v", lista.Clientes)
	}
	lista, _ = repo.Listar(ctx, Filtro{IncluirInativos: true, Busca: "aparecida"})
	if lista.Total != 1 {
		t.Errorf("busca incluindo inativos: total = %d", lista.Total)
	}
}
