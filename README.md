# autoReboque — API

Backend do **autoReboque**, sistema de orçamentos e ordens de serviço para
empresas de guincho/reboque. Produto genérico (white-label leve): cada
guincheira configura logo, dados, serviços, preços, clientes e motoristas.
Nasceu para a Auto Socorro Trevo (BA).

Go 1.27 + PostgreSQL 17. Arquitetura hexagonal com CQRS, biblioteca padrão
primeiro. O frontend é outro repositório:
[`autoReboque-web`](https://github.com/Gustavo-Resende/autoReboque-web).

> **Em transição.** O código em `internal/` é a versão anterior (monólito com
> `html/template` + HTMX) e está sendo substituído pela arquitetura descrita
> em [`CLAUDE.md`](CLAUDE.md), seguindo o plano em
> [`docs/planos/`](docs/planos/2026-09-18-arquitetura-hexagonal.md).

## O que o sistema faz

- **Orçamento → OS** no mesmo registro e número (`0142/2026`): aberto,
  enviado, expirado, recusado → agendada, em andamento, concluída, cancelada.
- **Clientes** (com cadastro rápido dentro da OS e o cliente padrão "Serviço
  particular"), **serviços** (catálogo com preço e unidade), **motoristas**,
  **empresa** (dados e identidade só nos documentos).
- **Fotos** por etapa (retirada/entrega), **documento** para impressão A4,
  resumo para **WhatsApp**, **visão geral** com números do mês.
- Edição concorrente segura (optimistic locking); dinheiro em centavos.

## Subindo em desenvolvimento

Pré-requisitos: Go 1.27+, Docker Desktop.

```sh
docker compose up -d db pgadmin     # Postgres + pgAdmin (http://localhost:5050)
cp .env.example .env                # ajuste se precisar
go run ./cmd/server                 # versão atual (legado) em http://localhost:8080
go run ./cmd/seed                   # (opcional) dados fictícios
```

Tudo no Docker, inclusive a API: `docker compose up -d --build`.

pgAdmin já vem com o servidor "autoReboque (docker)" cadastrado; senha do
banco: `autoreboque`.

## Testes

```sh
go test ./...                       # unitários

# com integração (banco descartável autoreboque_test, criado pelo compose):
$env:TEST_DATABASE_URL = 'postgres://autoreboque:autoreboque@localhost:5432/autoreboque_test?sslmode=disable'
go test -p 1 ./...                  # -p 1: os pacotes compartilham o banco
```

Nunca aponte `TEST_DATABASE_URL` para o banco de uso real: os testes apagam
as tabelas.

## Estrutura (alvo)

```
cmd/api                   composição e subida do servidor HTTP
cmd/seed                  dados de desenvolvimento
internal/domain           regras de negócio: um pacote por agregado, ports de repositório
internal/application      casos de uso (CQRS): commands, queries, dispatcher, ports
internal/adapters/http    rotas (chi), handlers magros, DTOs, middlewares
internal/adapters/postgres repositórios, SQL, migrações, transação
pkg                       ferramentas transversais: erros, validar, httpx, config, logger
docs                      decisões e planos
.claude/skills            skills por camada, para manter o padrão
```

Guia completo de arquitetura, convenções e regras: [`CLAUDE.md`](CLAUDE.md).

## Variáveis de ambiente

| Variável | Padrão | Uso |
|---|---|---|
| `DATABASE_URL` | — | conexão Postgres (obrigatória) |
| `HTTP_ADDR` | `:8080` | endereço do servidor |
| `CORS_ORIGENS` | `http://localhost:5173` | origens do frontend, separadas por vírgula (API nova) |
| `SESSAO_DURACAO` | `720h` | validade do login |
| `OS_NUMERO_INICIAL` | `1` | primeiro número emitido (só na primeira sequência) |
| `STORAGE_DIR` | `./dados/uploads` | pasta das imagens |
| `UPLOAD_MAX_MB` | `15` | limite por imagem |
| `DEV` | `false` | logs em texto, recarga de templates (legado) |
| `SENHA_HASH` | — | senha única do legado (some com ele) |
| `COOKIE_SECURE` | `false` | legado |
| `TEST_DATABASE_URL` | — | só para `go test` |
