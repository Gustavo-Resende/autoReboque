package ordemservico

import (
	"fmt"
	"strconv"
	"strings"
)

// Dinheiro é sempre inteiro em centavos: R$ 1.234,56 = 123456.
// Quantidades seguem a mesma ideia em centésimos: 1,5 h = 150.
// Nunca float — 0,1 + 0,2 não dá 0,3 em ponto flutuante.

// FormatarReais formata centavos no padrão brasileiro com prefixo: "R$ 1.234,56".
func FormatarReais(centavos int64) string {
	if centavos < 0 {
		return "-R$ " + FormatarValor(-centavos)
	}
	return "R$ " + FormatarValor(centavos)
}

// FormatarValor formata centavos sem prefixo: "1.234,56". Negativos ganham "-".
func FormatarValor(centavos int64) string {
	sinal := ""
	if centavos < 0 {
		sinal = "-"
		centavos = -centavos
	}
	return sinal + agruparMilhares(centavos/100) + "," + fmt.Sprintf("%02d", centavos%100)
}

// FormatarQuantidade formata centésimos sem casas decimais desnecessárias:
// 300 → "3", 150 → "1,5", 125 → "1,25".
func FormatarQuantidade(centesimos int64) string {
	inteiro := centesimos / 100
	resto := centesimos % 100
	if resto == 0 {
		return strconv.FormatInt(inteiro, 10)
	}
	s := fmt.Sprintf("%d,%02d", inteiro, resto)
	return strings.TrimSuffix(s, "0")
}

// ParseCentavos interpreta um valor digitado por pessoa em centavos.
// Aceita "1.234,56", "1234,56", "1234.56", "1234", "R$ 50" e espaços.
//
// Regra para o ponto: se houver vírgula, o ponto é separador de milhar.
// Sem vírgula, um único ponto seguido de exatamente 3 dígitos é milhar
// ("1.500" = 1500); qualquer outro caso é decimal ("12.5" = 12,50).
func ParseCentavos(texto string) (int64, error) {
	return parseDecimal(texto, "valor")
}

// ParseQuantidade interpreta uma quantidade em centésimos com as mesmas regras.
func ParseQuantidade(texto string) (int64, error) {
	return parseDecimal(texto, "quantidade")
}

func parseDecimal(texto, oQue string) (int64, error) {
	s := strings.ReplaceAll(strings.TrimSpace(texto), " ", "")
	// Aceita o sinal antes ou depois do "R$": "-R$ 5" e "R$ -5".
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "R$")
	if strings.HasPrefix(s, "-") {
		negativo = true
		s = strings.TrimPrefix(s, "-")
	}
	if s == "" {
		return 0, nil
	}

	var inteiro, decimal string
	switch {
	case strings.Contains(s, ","):
		if strings.Count(s, ",") > 1 {
			return 0, fmt.Errorf("%s inválido: %q", oQue, texto)
		}
		inteiro, decimal, _ = strings.Cut(s, ",")
		inteiro = strings.ReplaceAll(inteiro, ".", "")
	case strings.Count(s, ".") == 1:
		antes, depois, _ := strings.Cut(s, ".")
		if len(depois) == 3 {
			inteiro = antes + depois // milhar: 1.500
		} else {
			inteiro, decimal = antes, depois
		}
	default:
		inteiro = strings.ReplaceAll(s, ".", "")
	}

	if inteiro == "" {
		inteiro = "0"
	}
	if len(decimal) > 2 {
		return 0, fmt.Errorf("%s inválido: %q (no máximo 2 casas decimais)", oQue, texto)
	}
	decimal += strings.Repeat("0", 2-len(decimal))

	n, err := strconv.ParseInt(inteiro+decimal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %q", oQue, texto)
	}
	if negativo {
		n = -n
	}
	return n, nil
}

func agruparMilhares(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var partes []string
	for len(s) > 3 {
		partes = append([]string{s[len(s)-3:]}, partes...)
		s = s[:len(s)-3]
	}
	return s + "." + strings.Join(partes, ".")
}
