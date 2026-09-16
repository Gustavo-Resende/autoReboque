package imagem

import "encoding/binary"

// orientacaoJPEG lê a tag EXIF Orientation (0x0112) de um JPEG.
// Devolve 1 (normal) se não houver EXIF ou se algo não bater.
//
// Um JPEG é uma sequência de segmentos "FF xx [tamanho] [dados]". O EXIF mora
// no segmento APP1 (FF E1), começa com "Exif\0\0" e depois vem um bloco TIFF:
// ordem de bytes ("II" ou "MM"), o número 42, o deslocamento do primeiro IFD,
// e o IFD em si: uma contagem seguida de entradas de 12 bytes (tag, tipo,
// quantidade, valor). Só precisamos achar a entrada com tag 0x0112.
func orientacaoJPEG(dados []byte) int {
	const normal = 1
	if len(dados) < 4 || dados[0] != 0xFF || dados[1] != 0xD8 {
		return normal
	}

	pos := 2
	for pos+4 <= len(dados) {
		if dados[pos] != 0xFF {
			return normal
		}
		marcador := dados[pos+1]
		// Marcadores sem carga (padding e SOI/EOI): pula.
		if marcador == 0xFF {
			pos++
			continue
		}
		if marcador == 0xD8 || marcador == 0xD9 || (marcador >= 0xD0 && marcador <= 0xD7) {
			pos += 2
			continue
		}
		tamanho := int(binary.BigEndian.Uint16(dados[pos+2:]))
		if tamanho < 2 || pos+2+tamanho > len(dados) {
			return normal
		}
		// Chegou nos dados da imagem (SOS): não há mais EXIF depois.
		if marcador == 0xDA {
			return normal
		}
		if marcador == 0xE1 {
			carga := dados[pos+4 : pos+2+tamanho]
			if o := orientacaoDoExif(carga); o != 0 {
				return o
			}
		}
		pos += 2 + tamanho
	}
	return normal
}

// orientacaoDoExif interpreta a carga de um segmento APP1. Devolve 0 se não achar.
func orientacaoDoExif(carga []byte) int {
	if len(carga) < 6+8 || string(carga[:6]) != "Exif\x00\x00" {
		return 0
	}
	tiff := carga[6:]

	var ordem binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		ordem = binary.LittleEndian
	case "MM":
		ordem = binary.BigEndian
	default:
		return 0
	}
	if ordem.Uint16(tiff[2:]) != 42 {
		return 0
	}

	ifd := int(ordem.Uint32(tiff[4:]))
	if ifd+2 > len(tiff) {
		return 0
	}
	entradas := int(ordem.Uint16(tiff[ifd:]))
	for i := range entradas {
		inicio := ifd + 2 + i*12
		if inicio+12 > len(tiff) {
			return 0
		}
		tag := ordem.Uint16(tiff[inicio:])
		tipo := ordem.Uint16(tiff[inicio+2:])
		if tag == 0x0112 && tipo == 3 { // 3 = SHORT: o valor cabe nos 2 primeiros bytes
			o := int(ordem.Uint16(tiff[inicio+8:]))
			if o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}
