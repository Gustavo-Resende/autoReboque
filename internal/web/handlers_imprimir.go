package web

import (
	"net/http"

	"github.com/Gustavo-Resende/autoSocorro/internal/imagem"
)

// grupoDeFotos é um bloco de fotos de uma mesma etapa, para a impressão.
type grupoDeFotos struct {
	Etapa   imagem.Etapa
	Imagens []imagem.Imagem
}

// agruparPorEtapa separa a lista (já ordenada por etapa) em grupos.
func agruparPorEtapa(imagens []imagem.Imagem) []grupoDeFotos {
	var grupos []grupoDeFotos
	for _, img := range imagens {
		if len(grupos) == 0 || grupos[len(grupos)-1].Etapa != img.Etapa {
			grupos = append(grupos, grupoDeFotos{Etapa: img.Etapa})
		}
		grupos[len(grupos)-1].Imagens = append(grupos[len(grupos)-1].Imagens, img)
	}
	return grupos
}

// osImprimir é a página de impressão: sem o layout do sistema, com o CSS de
// impressão. A pessoa usa o Ctrl+P do navegador. O template é escolhido pelo
// modelo de documento — hoje só existe print/os.html.
func (s *Servidor) osImprimir(w http.ResponseWriter, r *http.Request) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	imagens, err := s.imagens.ListarDaOS(r.Context(), os.ID)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	emp, err := s.empresa.Buscar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	// Fotos agrupadas por etapa, na ordem retirada → entrega → outras.
	grupos := agruparPorEtapa(imagens)
	d := dados{
		"OS":      os,
		"Imagens": imagens,
		"Grupos":  grupos,
		"Empresa": emp,
		"Auto":    r.URL.Query().Get("auto") == "1", // abre a caixa de impressão sozinho
	}
	if err := s.tpl.RenderizarBloco(w, http.StatusOK, "print/os.html", "impressao", d); err != nil {
		s.erroInterno(w, r, err)
	}
}
