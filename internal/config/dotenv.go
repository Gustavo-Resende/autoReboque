package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// carregarDotEnv lê um arquivo no formato CHAVE=valor e define cada variável
// no ambiente do processo, sem sobrescrever as que já existem. Linhas vazias
// e começando com # são ignoradas. Se o arquivo não existir, não é erro.
//
// É deliberadamente simples: sem interpolação, sem múltiplas linhas.
func carregarDotEnv(caminho string) error {
	f, err := os.Open(caminho)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	linha := 0
	for scanner.Scan() {
		linha++
		texto := strings.TrimSpace(scanner.Text())
		if texto == "" || strings.HasPrefix(texto, "#") {
			continue
		}
		chave, valor, ok := strings.Cut(texto, "=")
		if !ok {
			return fmt.Errorf("%s:%d: linha inválida (esperado CHAVE=valor)", caminho, linha)
		}
		chave = strings.TrimSpace(chave)
		valor = strings.TrimSpace(valor)
		// Aceita valores entre aspas, como em VAR="algo com espaço".
		if len(valor) >= 2 && (valor[0] == '"' && valor[len(valor)-1] == '"' || valor[0] == '\'' && valor[len(valor)-1] == '\'') {
			valor = valor[1 : len(valor)-1]
		}
		if _, definida := os.LookupEnv(chave); !definida {
			if err := os.Setenv(chave, valor); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
