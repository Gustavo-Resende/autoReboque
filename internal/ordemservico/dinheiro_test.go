package ordemservico

import "testing"

func TestFormatarReais(t *testing.T) {
	casos := []struct {
		centavos int64
		esperado string
	}{
		{0, "R$ 0,00"},
		{5, "R$ 0,05"},
		{100, "R$ 1,00"},
		{123456, "R$ 1.234,56"},
		{100000000, "R$ 1.000.000,00"},
		{-1050, "-R$ 10,50"},
	}
	for _, c := range casos {
		if got := FormatarReais(c.centavos); got != c.esperado {
			t.Errorf("FormatarReais(%d) = %q, esperado %q", c.centavos, got, c.esperado)
		}
	}
}

func TestFormatarQuantidade(t *testing.T) {
	casos := []struct {
		centesimos int64
		esperado   string
	}{
		{0, "0"},
		{100, "1"},
		{150, "1,5"},
		{125, "1,25"},
		{3700, "37"},
		{1005, "10,05"},
	}
	for _, c := range casos {
		if got := FormatarQuantidade(c.centesimos); got != c.esperado {
			t.Errorf("FormatarQuantidade(%d) = %q, esperado %q", c.centesimos, got, c.esperado)
		}
	}
}

func TestParseCentavos(t *testing.T) {
	casos := []struct {
		texto    string
		esperado int64
	}{
		{"", 0},
		{"0", 0},
		{"150", 15000},
		{"150,00", 15000},
		{"150,5", 15050},
		{"1.234,56", 123456},
		{"1234,56", 123456},
		{"1234.56", 123456},
		{"12.5", 1250},
		{"1.500", 150000}, // ponto + 3 dígitos = milhar
		{"1.500.000", 150000000},
		{"R$ 50", 5000},
		{"R$ 1.234,56", 123456},
		{" 99,9 ", 9990},
		{"-20", -2000},
		{",50", 50},
	}
	for _, c := range casos {
		got, err := ParseCentavos(c.texto)
		if err != nil {
			t.Errorf("ParseCentavos(%q): erro inesperado: %v", c.texto, err)
			continue
		}
		if got != c.esperado {
			t.Errorf("ParseCentavos(%q) = %d, esperado %d", c.texto, got, c.esperado)
		}
	}

	invalidos := []string{"abc", "1,2,3", "12,345", "1.2.3,4x"}
	for _, texto := range invalidos {
		if _, err := ParseCentavos(texto); err == nil {
			t.Errorf("ParseCentavos(%q): esperado erro", texto)
		}
	}
}

// Formatar e depois interpretar tem que devolver o mesmo valor.
func TestParseCentavosIdaEVolta(t *testing.T) {
	for _, v := range []int64{0, 1, 99, 100, 123456, 100000000, -5050} {
		got, err := ParseCentavos(FormatarReais(v))
		if err != nil || got != v {
			t.Errorf("ida e volta de %d: got %d, err %v", v, got, err)
		}
	}
}
