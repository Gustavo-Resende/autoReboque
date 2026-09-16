package ordemservico

import "fmt"

// Numero identifica uma OS para as pessoas: sequencial dentro do ano.
// O id da tabela é o que o sistema usa nas URLs; o Numero é o que vai impresso.
type Numero struct {
	Sequencial int
	Ano        int
}

// String formata como 0142/2026. Passando de 9999 o número simplesmente cresce.
func (n Numero) String() string {
	return fmt.Sprintf("%04d/%d", n.Sequencial, n.Ano)
}
