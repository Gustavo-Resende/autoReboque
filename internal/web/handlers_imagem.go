package web

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Gustavo-Resende/autoSocorro/internal/imagem"
	"github.com/Gustavo-Resende/autoSocorro/internal/ordemservico"
	"github.com/Gustavo-Resende/autoSocorro/internal/storage"
)

func etapasDeFoto() []imagem.Etapa { return imagem.TodasEtapas }

// imagemEnviar recebe uma ou mais fotos (campo "fotos", multipart) com a etapa.
// Responde com o bloco de imagens atualizado (HTMX) ou redireciona.
func (s *Servidor) imagemEnviar(w http.ResponseWriter, r *http.Request) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return
	}
	// O limite por arquivo é verificado pelo serviço; aqui limitamos a
	// requisição inteira para ninguém derrubar o servidor com um POST gigante.
	r.Body = http.MaxBytesReader(w, r.Body, 10*s.cfg.UploadMaxBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.responderImagens(w, r, os.ID, http.StatusRequestEntityTooLarge, "Envio muito grande.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	etapa := imagem.Etapa(r.FormValue("etapa"))
	arquivos := r.MultipartForm.File["fotos"]
	if len(arquivos) == 0 {
		s.responderImagens(w, r, os.ID, http.StatusUnprocessableEntity, "Selecione pelo menos uma foto.")
		return
	}

	var mensagemErro string
	enviadas := 0
	for _, cabecalho := range arquivos {
		f, err := cabecalho.Open()
		if err != nil {
			mensagemErro = "Não foi possível ler um dos arquivos."
			continue
		}
		_, err = s.imagens.Anexar(r.Context(), os.ID, etapa, cabecalho.Filename, f)
		f.Close()
		switch {
		case err == nil:
			enviadas++
		case errors.Is(err, imagem.ErrFormatoNaoSuportado), errors.Is(err, imagem.ErrArquivoGrande):
			mensagemErro = cabecalho.Filename + ": " + err.Error()
		default:
			slog.Error("anexar imagem", "os", os.ID, "arquivo", cabecalho.Filename, "erro", err)
			mensagemErro = cabecalho.Filename + ": erro ao salvar."
		}
	}

	status := http.StatusOK
	if mensagemErro != "" && enviadas == 0 {
		status = http.StatusUnprocessableEntity
	}
	s.responderImagens(w, r, os.ID, status, mensagemErro)
}

// imagemEtapa muda a etapa (retirada/entrega/outra) de uma foto.
func (s *Servidor) imagemEtapa(w http.ResponseWriter, r *http.Request) {
	os, img, ok := s.carregarImagemDaOS(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	if err := s.imagens.AlterarEtapa(r.Context(), img.ID, imagem.Etapa(r.PostForm.Get("etapa"))); err != nil {
		s.responderImagens(w, r, os.ID, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.responderImagens(w, r, os.ID, http.StatusOK, "")
}

func (s *Servidor) imagemRemover(w http.ResponseWriter, r *http.Request) {
	os, img, ok := s.carregarImagemDaOS(w, r)
	if !ok {
		return
	}
	if err := s.imagens.Remover(r.Context(), img.ID); err != nil {
		s.erroInterno(w, r, err)
		return
	}
	s.responderImagens(w, r, os.ID, http.StatusOK, "")
}

// carregarImagemDaOS garante que a imagem pertence à OS da rota.
func (s *Servidor) carregarImagemDaOS(w http.ResponseWriter, r *http.Request) (*ordemservico.OS, *imagem.Imagem, bool) {
	os, ok := s.carregarOS(w, r)
	if !ok {
		return nil, nil, false
	}
	imgID, ok := idDaRota(r, "imagemID")
	if !ok {
		s.erro(w, r, http.StatusNotFound, "Imagem não encontrada.")
		return nil, nil, false
	}
	img, err := s.imagens.Buscar(r.Context(), imgID)
	if err != nil || img.OrdemServicoID != os.ID {
		s.erro(w, r, http.StatusNotFound, "Imagem não encontrada.")
		return nil, nil, false
	}
	return os, img, true
}

// responderImagens renderiza o bloco de fotos da tela de detalhe.
func (s *Servidor) responderImagens(w http.ResponseWriter, r *http.Request, osID int64, status int, mensagemErro string) {
	if !ehHTMX(r) {
		if mensagemErro != "" {
			definirFlash(w, mensagemErro)
		}
		http.Redirect(w, r, fmt.Sprintf("/os/%d", osID), http.StatusSeeOther)
		return
	}
	imagens, err := s.imagens.ListarDaOS(r.Context(), osID)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	d := dados{"OSID": osID, "Imagens": imagens, "Etapas": etapasDeFoto(), "Erro": mensagemErro}
	if err := s.tpl.RenderizarBloco(w, status, "pages/os_detalhe.html", "os_imagens", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

func (s *Servidor) imagemOriginal(w http.ResponseWriter, r *http.Request) {
	s.servirImagem(w, r, false)
}

func (s *Servidor) imagemMiniatura(w http.ResponseWriter, r *http.Request) {
	s.servirImagem(w, r, true)
}

// servirImagem entrega o arquivo lendo pelo Storage (e não direto do disco),
// para que trocar o Storage no futuro não mude nada aqui.
func (s *Servidor) servirImagem(w http.ResponseWriter, r *http.Request, miniatura bool) {
	imgID, ok := idDaRota(r, "imagemID")
	if !ok {
		http.NotFound(w, r)
		return
	}
	img, err := s.imagens.Buscar(r.Context(), imgID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	chave, tipo := img.ChaveOriginal, img.ContentType
	if miniatura {
		chave, tipo = img.ChaveMiniatura, "image/jpeg"
	}
	conteudo, err := s.imagens.Abrir(r.Context(), chave)
	if errors.Is(err, storage.ErrNaoEncontrado) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	defer conteudo.Close()

	w.Header().Set("Content-Type", tipo)
	// As chaves nunca mudam de conteúdo, então o navegador pode guardar à vontade.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if !miniatura {
		w.Header().Set("Content-Length", strconv.FormatInt(img.TamanhoBytes, 10))
	}
	io.Copy(w, conteudo)
}
