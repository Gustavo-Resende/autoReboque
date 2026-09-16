package ordemservico

// Item é uma linha da OS: "Saída de base", "Quilometragem rodada" etc.
//
// Pode ter vindo do catálogo (ServicoID) ou ter sido digitado livremente.
// Descrição, unidade e preço ficam copiados aqui: mudar o catálogo depois não
// altera OS antigas.
type Item struct {
	ServicoID int64 // 0 = item livre
	Descricao string
	Unidade   string // UN, KM, HORA, DIARIA

	// QuantidadeCentesimos guarda a quantidade × 100: 37 km = 3700, 1,5 h = 150.
	QuantidadeCentesimos int64

	ValorUnitarioCentavos int64
}

// Subtotal é quantidade × valor unitário, em centavos.
//
// O produto de centésimos por centavos dá décimos de milésimo; dividimos por
// 100 arredondando para o centavo mais próximo (meio para cima em módulo).
// Tudo em inteiros: nenhuma etapa passa por float.
func (i Item) Subtotal() int64 {
	produto := i.QuantidadeCentesimos * i.ValorUnitarioCentavos
	if produto >= 0 {
		return (produto + 50) / 100
	}
	return (produto - 50) / 100
}

// Vazio diz se a linha não tem nada preenchido (formulários mandam linhas em branco).
func (i Item) Vazio() bool {
	return i.Descricao == "" && i.QuantidadeCentesimos == 0 && i.ValorUnitarioCentavos == 0
}

// RotuloUnidade é a unidade para exibição ("h", "km", "un", "diária").
func (i Item) RotuloUnidade() string {
	return RotuloUnidade(i.Unidade)
}

// RotuloUnidade converte o código da unidade no texto curto de exibição.
func RotuloUnidade(unidade string) string {
	switch unidade {
	case "KM":
		return "km"
	case "HORA":
		return "h"
	case "DIARIA":
		return "diária"
	default:
		return "un"
	}
}

// SomarItens devolve o total em centavos de uma lista de itens.
func SomarItens(itens []Item) int64 {
	var total int64
	for _, it := range itens {
		total += it.Subtotal()
	}
	return total
}
