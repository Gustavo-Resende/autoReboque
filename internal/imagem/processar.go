package imagem

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"

	"golang.org/x/image/draw"

	// Registram os decodificadores em image.Decode.
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// ErrFormatoNaoSuportado é devolvido para arquivos que não são JPEG/PNG/WebP.
// HEIC (padrão em iPhone) não é suportado pelas bibliotecas do Go; a mensagem
// orienta a pessoa a mudar a configuração da câmera.
var ErrFormatoNaoSuportado = errors.New(
	"formato de imagem não suportado: envie JPEG, PNG ou WebP " +
		"(no iPhone: Ajustes → Câmera → Formatos → Mais compatível)")

// Formato descreve um arquivo de imagem aceito.
type Formato struct {
	ContentType string // image/jpeg, image/png, image/webp
	Extensao    string // .jpg, .png, .webp
}

var formatosAceitos = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// Detectar identifica o formato pelo conteúdo (não pelo nome do arquivo) e
// confirma que a imagem decodifica. É a validação usada em todo upload.
func Detectar(dados []byte) (Formato, error) {
	tipo := http.DetectContentType(dados)
	ext, ok := formatosAceitos[tipo]
	if !ok {
		return Formato{}, ErrFormatoNaoSuportado
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(dados)); err != nil {
		return Formato{}, ErrFormatoNaoSuportado
	}
	return Formato{ContentType: tipo, Extensao: ext}, nil
}

// ladoMaximoMiniatura é o tamanho da caixa em que a miniatura cabe.
// 480px é suficiente para identificar o veículo na tela e na impressão.
const ladoMaximoMiniatura = 480

// GerarMiniatura decodifica a imagem, reduz para caber em ladoMaximoMiniatura
// (mantendo a proporção), corrige a orientação EXIF e devolve um JPEG.
func GerarMiniatura(dados []byte) ([]byte, error) {
	original, formato, err := image.Decode(bytes.NewReader(dados))
	if err != nil {
		return nil, fmt.Errorf("decodificar imagem: %w", err)
	}

	limites := original.Bounds()
	largura, altura := limites.Dx(), limites.Dy()
	if largura == 0 || altura == 0 {
		return nil, errors.New("imagem vazia")
	}

	// Reduz primeiro (barato de girar uma imagem pequena depois).
	novaLargura, novaAltura := largura, altura
	if largura > ladoMaximoMiniatura || altura > ladoMaximoMiniatura {
		if largura >= altura {
			novaLargura = ladoMaximoMiniatura
			novaAltura = max(1, altura*ladoMaximoMiniatura/largura)
		} else {
			novaAltura = ladoMaximoMiniatura
			novaLargura = max(1, largura*ladoMaximoMiniatura/altura)
		}
	}
	reduzida := image.NewNRGBA(image.Rect(0, 0, novaLargura, novaAltura))
	draw.CatmullRom.Scale(reduzida, reduzida.Bounds(), original, limites, draw.Over, nil)

	var resultado image.Image = reduzida
	if formato == "jpeg" {
		resultado = corrigirOrientacao(reduzida, orientacaoJPEG(dados))
	}

	var saida bytes.Buffer
	if err := jpeg.Encode(&saida, resultado, &jpeg.Options{Quality: 80}); err != nil {
		return nil, fmt.Errorf("codificar miniatura: %w", err)
	}
	return saida.Bytes(), nil
}

// corrigirOrientacao aplica a rotação/espelhamento indicados pela tag EXIF
// Orientation (1 a 8). Câmeras de celular costumam gravar a foto "deitada"
// e marcar a orientação real na tag; sem isso a miniatura sairia de lado.
func corrigirOrientacao(img *image.NRGBA, orientacao int) image.Image {
	if orientacao <= 1 || orientacao > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// Para cada orientação: tamanho de saída e a função que diz, para um
	// pixel (x, y) da saída, de onde ele vem na entrada.
	var saidaW, saidaH int
	var origem func(x, y int) (int, int)
	switch orientacao {
	case 2: // espelho horizontal
		saidaW, saidaH = w, h
		origem = func(x, y int) (int, int) { return w - 1 - x, y }
	case 3: // 180°
		saidaW, saidaH = w, h
		origem = func(x, y int) (int, int) { return w - 1 - x, h - 1 - y }
	case 4: // espelho vertical
		saidaW, saidaH = w, h
		origem = func(x, y int) (int, int) { return x, h - 1 - y }
	case 5: // transposta
		saidaW, saidaH = h, w
		origem = func(x, y int) (int, int) { return y, x }
	case 6: // 90° horário
		saidaW, saidaH = h, w
		origem = func(x, y int) (int, int) { return y, h - 1 - x }
	case 7: // transversa
		saidaW, saidaH = h, w
		origem = func(x, y int) (int, int) { return w - 1 - y, h - 1 - x }
	case 8: // 90° anti-horário
		saidaW, saidaH = h, w
		origem = func(x, y int) (int, int) { return w - 1 - y, x }
	}

	saida := image.NewNRGBA(image.Rect(0, 0, saidaW, saidaH))
	for y := 0; y < saidaH; y++ {
		for x := 0; x < saidaW; x++ {
			ox, oy := origem(x, y)
			saida.SetNRGBA(x, y, img.NRGBAAt(b.Min.X+ox, b.Min.Y+oy))
		}
	}
	return saida
}
