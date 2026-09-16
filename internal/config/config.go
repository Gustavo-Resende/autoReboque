// Package config lê a configuração do sistema a partir de variáveis de ambiente.
//
// Toda configuração passa por aqui: nenhum outro pacote chama os.Getenv.
// Assim fica fácil saber o que o sistema precisa para subir e com quais padrões.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config guarda tudo que o sistema precisa saber para subir.
type Config struct {
	DatabaseURL string
	HTTPAddr    string

	// SenhaHash é o hash bcrypt da senha única de acesso (ver cmd/hashsenha).
	SenhaHash string

	// StorageDir é a pasta onde as imagens ficam em disco.
	StorageDir string

	// OSNumeroInicial é o primeiro número emitido pelo sistema.
	// Vale só para a primeira sequência criada; anos seguintes começam em 1.
	OSNumeroInicial int

	UploadMaxBytes int64
	SessaoDuracao  time.Duration
	CookieSecure   bool

	// Dev liga conveniências de desenvolvimento (recarregar templates do disco).
	Dev bool
}

// Carregar monta a Config a partir do ambiente. Se existir um arquivo .env
// no diretório atual, ele é lido antes, sem sobrescrever variáveis já definidas.
func Carregar() (Config, error) {
	if err := carregarDotEnv(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		HTTPAddr:        valorOuPadrao("HTTP_ADDR", ":8080"),
		SenhaHash:       os.Getenv("SENHA_HASH"),
		StorageDir:      valorOuPadrao("STORAGE_DIR", "./dados/uploads"),
		OSNumeroInicial: 1,
		UploadMaxBytes:  15 << 20,
		SessaoDuracao:   30 * 24 * time.Hour,
	}

	var err error
	if cfg.OSNumeroInicial, err = inteiro("OS_NUMERO_INICIAL", cfg.OSNumeroInicial); err != nil {
		return Config{}, err
	}
	maxMB, err := inteiro("UPLOAD_MAX_MB", 15)
	if err != nil {
		return Config{}, err
	}
	cfg.UploadMaxBytes = int64(maxMB) << 20
	if cfg.SessaoDuracao, err = duracao("SESSAO_DURACAO", cfg.SessaoDuracao); err != nil {
		return Config{}, err
	}
	if cfg.CookieSecure, err = booleano("COOKIE_SECURE", false); err != nil {
		return Config{}, err
	}
	if cfg.Dev, err = booleano("DEV", false); err != nil {
		return Config{}, err
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL não definida")
	}
	if cfg.SenhaHash == "" {
		return Config{}, errors.New("SENHA_HASH não definida (gere com: go run ./cmd/hashsenha)")
	}
	if cfg.OSNumeroInicial < 1 {
		return Config{}, errors.New("OS_NUMERO_INICIAL deve ser maior que zero")
	}
	return cfg, nil
}

func valorOuPadrao(chave, padrao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return padrao
}

func inteiro(chave string, padrao int) (int, error) {
	v := os.Getenv(chave)
	if v == "" {
		return padrao, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: esperado número inteiro, veio %q", chave, v)
	}
	return n, nil
}

func booleano(chave string, padrao bool) (bool, error) {
	v := os.Getenv(chave)
	if v == "" {
		return padrao, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: esperado true/false, veio %q", chave, v)
	}
	return b, nil
}

func duracao(chave string, padrao time.Duration) (time.Duration, error) {
	v := os.Getenv(chave)
	if v == "" {
		return padrao, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: esperado duração como 720h ou 30m, veio %q", chave, v)
	}
	return d, nil
}
