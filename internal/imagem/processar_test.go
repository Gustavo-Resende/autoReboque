package imagem

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// imagemDeTeste cria uma imagem largura×altura com o canto superior esquerdo
// vermelho e o resto branco, para dar para conferir a orientação depois.
func imagemDeTeste(largura, altura int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, largura, altura))
	for y := range altura {
		for x := range largura {
			c := color.NRGBA{255, 255, 255, 255}
			if x < largura/4 && y < altura/4 {
				c = color.NRGBA{255, 0, 0, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestDetectar(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, imagemDeTeste(10, 10)); err != nil {
		t.Fatal(err)
	}
	f, err := Detectar(buf.Bytes())
	if err != nil || f.ContentType != "image/png" || f.Extensao != ".png" {
		t.Errorf("png: %+v, %v", f, err)
	}

	if _, err := Detectar([]byte("isso não é uma imagem")); err == nil {
		t.Error("texto deveria ser recusado")
	}
	if _, err := Detectar([]byte("%PDF-1.4 ...")); err == nil {
		t.Error("pdf deveria ser recusado")
	}
}

func TestGerarMiniaturaReduzMantendoProporcao(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imagemDeTeste(2000, 1000), nil); err != nil {
		t.Fatal(err)
	}
	mini, err := GerarMiniatura(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, formato, err := image.DecodeConfig(bytes.NewReader(mini))
	if err != nil {
		t.Fatal(err)
	}
	if formato != "jpeg" {
		t.Errorf("miniatura deveria ser jpeg, veio %s", formato)
	}
	if cfg.Width != ladoMaximoMiniatura || cfg.Height != ladoMaximoMiniatura/2 {
		t.Errorf("miniatura = %dx%d, esperado %dx%d", cfg.Width, cfg.Height, ladoMaximoMiniatura, ladoMaximoMiniatura/2)
	}

	// Imagem já pequena não é ampliada.
	buf.Reset()
	if err := png.Encode(&buf, imagemDeTeste(100, 50)); err != nil {
		t.Fatal(err)
	}
	mini, _ = GerarMiniatura(buf.Bytes())
	cfg, _, _ = image.DecodeConfig(bytes.NewReader(mini))
	if cfg.Width != 100 || cfg.Height != 50 {
		t.Errorf("imagem pequena = %dx%d, esperado 100x50", cfg.Width, cfg.Height)
	}
}

// jpegComOrientacao monta um JPEG com um segmento APP1/EXIF contendo só a
// tag Orientation, inserido logo depois do SOI.
func jpegComOrientacao(t *testing.T, img image.Image, orientacao uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	original := buf.Bytes()

	// Bloco TIFF big-endian: cabeçalho (8 bytes) + IFD0 com 1 entrada.
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8}
	tiff = binary.BigEndian.AppendUint16(tiff, 1)          // 1 entrada
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)     // tag Orientation
	tiff = binary.BigEndian.AppendUint16(tiff, 3)          // tipo SHORT
	tiff = binary.BigEndian.AppendUint32(tiff, 1)          // 1 valor
	tiff = binary.BigEndian.AppendUint16(tiff, orientacao) // valor
	tiff = binary.BigEndian.AppendUint16(tiff, 0)          // preenchimento
	tiff = binary.BigEndian.AppendUint32(tiff, 0)          // próximo IFD: nenhum

	carga := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(carga)+2))
	app1 = append(app1, carga...)

	saida := append([]byte{}, original[:2]...) // SOI
	saida = append(saida, app1...)
	saida = append(saida, original[2:]...)
	return saida
}

func TestOrientacaoJPEG(t *testing.T) {
	img := imagemDeTeste(40, 20)
	if o := orientacaoJPEG(jpegComOrientacao(t, img, 6)); o != 6 {
		t.Errorf("orientação lida = %d, esperado 6", o)
	}
	var semExif bytes.Buffer
	_ = jpeg.Encode(&semExif, img, nil)
	if o := orientacaoJPEG(semExif.Bytes()); o != 1 {
		t.Errorf("sem EXIF: orientação = %d, esperado 1", o)
	}
	if o := orientacaoJPEG([]byte("lixo")); o != 1 {
		t.Errorf("lixo: orientação = %d, esperado 1", o)
	}
}

func TestGerarMiniaturaCorrigeOrientacao(t *testing.T) {
	// Foto "deitada" 40x20 marcada como girada 90° horário: a miniatura
	// deve sair 20x40, com o canto vermelho no canto superior DIREITO.
	dados := jpegComOrientacao(t, imagemDeTeste(40, 20), 6)
	mini, err := GerarMiniatura(dados)
	if err != nil {
		t.Fatal(err)
	}
	decodificada, err := jpeg.Decode(bytes.NewReader(mini))
	if err != nil {
		t.Fatal(err)
	}
	b := decodificada.Bounds()
	if b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("miniatura girada = %dx%d, esperado 20x40", b.Dx(), b.Dy())
	}
	r, g, _, _ := decodificada.At(18, 2).RGBA()
	if r < 0xC000 || g > 0x4000 {
		t.Errorf("canto superior direito deveria ser vermelho, veio r=%d g=%d", r>>8, g>>8)
	}
	r, g, _, _ = decodificada.At(2, 2).RGBA()
	if g < 0xC000 {
		t.Errorf("canto superior esquerdo deveria ser branco, veio r=%d g=%d", r>>8, g>>8)
	}
}
