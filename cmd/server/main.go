// server é o ponto de entrada do sistema: carrega a configuração, conecta ao
// banco, aplica as migrations, monta as dependências e sobe o HTTP.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Gustavo-Resende/autoSocorro/internal/auth"
	"github.com/Gustavo-Resende/autoSocorro/internal/cliente"
	"github.com/Gustavo-Resende/autoSocorro/internal/config"
	"github.com/Gustavo-Resende/autoSocorro/internal/database"
	"github.com/Gustavo-Resende/autoSocorro/internal/empresa"
	"github.com/Gustavo-Resende/autoSocorro/internal/imagem"
	"github.com/Gustavo-Resende/autoSocorro/internal/motorista"
	"github.com/Gustavo-Resende/autoSocorro/internal/ordemservico"
	"github.com/Gustavo-Resende/autoSocorro/internal/servico"
	"github.com/Gustavo-Resende/autoSocorro/internal/storage"
	"github.com/Gustavo-Resende/autoSocorro/internal/web"
)

// empresaAtual é a única empresa por enquanto. Quando o sistema atender mais
// de uma guincheira, este id passa a vir da sessão/domínio de quem entrou.
const empresaAtual int64 = 1

func main() {
	if err := executar(); err != nil {
		slog.Error("encerrando por erro", "erro", err)
		os.Exit(1)
	}
}

func executar() error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Carregar()
	if err != nil {
		return err
	}

	// Ctrl+C (ou SIGTERM do sistema) cancela este contexto e inicia o desligamento.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	pool, err := database.Conectar(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrar(ctx, pool); err != nil {
		return err
	}

	// Aqui é o único lugar que sabe qual Storage está em uso.
	// Para trocar por S3/R2 no futuro, muda-se só esta linha.
	arquivos, err := storage.NovoLocal(cfg.StorageDir)
	if err != nil {
		return err
	}

	servidor, err := web.NovoServidor(web.Dependencias{
		Config:     cfg,
		Ordens:     ordemservico.NovoRepositorio(pool, empresaAtual, cfg.OSNumeroInicial),
		Clientes:   cliente.NovoRepositorio(pool, empresaAtual),
		Servicos:   servico.NovoRepositorio(pool, empresaAtual),
		Motoristas: motorista.NovoRepositorio(pool, empresaAtual),
		Imagens:    imagem.NovoServico(pool, arquivos, cfg.UploadMaxBytes),
		Empresa:    empresa.NovoRepositorio(pool, empresaAtual, arquivos),
		Sessoes:    auth.NovasSessoes(pool, cfg.SessaoDuracao),
		Verificar: func() error {
			ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancelar()
			return pool.Ping(ctx)
		},
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           servidor.Rotas(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // uploads de fotos pelo 4G podem demorar
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	erroServidor := make(chan error, 1)
	go func() {
		slog.Info("servidor no ar", "endereco", cfg.HTTPAddr, "dev", cfg.Dev)
		erroServidor <- srv.ListenAndServe()
	}()

	select {
	case err := <-erroServidor:
		return err
	case <-ctx.Done():
		slog.Info("desligando...")
		ctxDesligar, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		if err := srv.Shutdown(ctxDesligar); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}
}
