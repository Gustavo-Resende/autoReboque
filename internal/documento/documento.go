// Package documento define os modelos de documento que o sistema emite.
//
// Na V1 só existe a Ordem de Serviço. Quando entrar outro modelo (ex.: Recibo),
// ele ganha uma constante aqui, uma sequência própria de numeração
// (sequencia_documento.tipo) e um template de impressão próprio. O núcleo da
// OS não muda.
package documento

// Tipo identifica um modelo de documento. O valor é o que vai gravado
// na coluna sequencia_documento.tipo.
type Tipo string

const (
	OS Tipo = "OS"
)

// Rotulo é o nome do modelo para exibição.
func (t Tipo) Rotulo() string {
	switch t {
	case OS:
		return "Ordem de Serviço"
	}
	return string(t)
}
