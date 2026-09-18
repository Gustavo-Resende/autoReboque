// hashsenha gera o hash bcrypt de uma senha para a variável SENHA_HASH.
//
//	go run ./cmd/hashsenha            (pede a senha no terminal)
//	go run ./cmd/hashsenha minhasenha
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Gustavo-Resende/autoReboque/internal/auth"
)

func main() {
	var senha string
	if len(os.Args) > 1 {
		senha = os.Args[1]
	} else {
		fmt.Fprint(os.Stderr, "Senha: ")
		linha, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && linha == "" {
			fmt.Fprintln(os.Stderr, "não foi possível ler a senha")
			os.Exit(1)
		}
		senha = strings.TrimRight(linha, "\r\n")
	}
	if len(senha) < 6 {
		fmt.Fprintln(os.Stderr, "a senha deve ter pelo menos 6 caracteres")
		os.Exit(1)
	}
	hash, err := auth.GerarHash(senha)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao gerar hash:", err)
		os.Exit(1)
	}
	// Entre aspas simples: o hash tem cifrões e assim vai direto para o .env.
	fmt.Printf("SENHA_HASH='%s'\n", hash)
}
