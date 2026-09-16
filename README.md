# Sistema de OS para guincho — Auto Socorro Trevo

Sistema de ordens de serviço e orçamentos para empresas de guincho/reboque.
Nasceu para a Auto Socorro Trevo e é configurável (logo, cor, dados, serviços,
preços, clientes) para servir outras guincheiras.

Go + PostgreSQL + `html/template` + HTMX. Sem framework, sem build de front.

## O que faz

- **Orçamento → OS** no mesmo registro e número (`0142/2026`): aberto, enviado,
  expirado, recusado → agendada, em andamento, concluída, cancelada.
- **Visão geral**: a receber, faturado/recebido no mês, orçamentos aguardando,
  gráfico de 6 meses, formas de pagamento. Só OS conta; orçamento é rascunho.
- **Clientes** (com cadastro rápido dentro da OS e o cliente padrão
  "Serviço particular"), **serviços** (catálogo com preço e unidade),
  **motoristas**.
- **Fotos** por etapa (retirada/entrega), **impressão** A4 em preto e branco,
  **WhatsApp** com resumo do orçamento.
- Tema claro/escuro, celular em primeiro lugar, edição concorrente segura
  (optimistic locking).

## Subindo em desenvolvimento

Pré-requisitos: Go 1.27+, Docker Desktop.

```sh
docker compose up -d            # Postgres (e o banco de testes autosocorro_test)
cp .env.example .env            # senha de exemplo: "trevo123"
go run ./cmd/server             # aplica as migrations e sobe em http://localhost:8080
go run ./cmd/seed               # (opcional) clientes, motoristas e 70 OS fictícias
```

Para trocar a senha: `go run ./cmd/hashsenha` imprime a linha `SENHA_HASH=...`
pronta para o `.env`. Com `DEV=true`, templates e CSS são relidos do disco a cada
requisição.

Para zerar o banco de desenvolvimento: `docker compose down -v` e suba de novo.

## Testes

```sh
go test ./...                    # unitários (sem banco)

# com os testes de integração (numeração concorrente, optimistic locking, listagem, dashboard):
export TEST_DATABASE_URL='postgres://autosocorro:autosocorro@localhost:5432/autosocorro_test?sslmode=disable'
go test -p 1 ./...               # -p 1: os pacotes compartilham o banco de teste
```

No PowerShell: `$env:TEST_DATABASE_URL = '...'`. Nunca aponte para o banco de uso
real: os testes apagam as tabelas.

## Estrutura

```
cmd/server             ponto de entrada (config → banco → migrations → HTTP)
cmd/seed               dados fictícios para testar telas e dashboard
cmd/hashsenha          gera o hash bcrypt da senha de acesso
internal/config        variáveis de ambiente (+ leitura do .env)
internal/database      pool pgx, migrator embutido (com advisory lock), migrations/*.sql
internal/documento     enum dos modelos de documento (hoje só OS)
internal/ordemservico  DOMÍNIO: OS/orçamento, itens, status, numeração, dinheiro, listagem, dashboard
internal/cliente       cadastro de clientes (+ cliente de sistema "Serviço particular")
internal/servico       catálogo de serviços
internal/motorista     motoristas
internal/empresa       dados, logo, cor e regras de orçamento da empresa
internal/auth          senha única, sessões em banco, middleware
internal/storage       interface Storage + implementação em disco
internal/imagem        upload, validação, miniatura (com correção EXIF), etapa
internal/web           handlers, templates, CSS, HTMX
```

Arquitetura atual: monólito em camadas por funcionalidade (repositório junto do
domínio). A reorganização para ports/casos de uso está planejada para depois que
o domínio estabilizar.

## Variáveis de ambiente

| Variável | Padrão | Uso |
|---|---|---|
| `DATABASE_URL` | — | conexão Postgres (obrigatória) |
| `SENHA_HASH` | — | hash bcrypt da senha, entre aspas simples (obrigatória) |
| `HTTP_ADDR` | `:8080` | endereço do servidor |
| `STORAGE_DIR` | `./dados/uploads` | pasta das imagens |
| `OS_NUMERO_INICIAL` | `1` | primeiro número emitido pelo sistema (só na primeira sequência) |
| `UPLOAD_MAX_MB` | `15` | limite por imagem |
| `SESSAO_DURACAO` | `720h` | validade do login |
| `COOKIE_SECURE` | `false` | `true` atrás de HTTPS |
| `DEV` | `false` | recarrega templates do disco |

## Decisões que vale saber

- **Dinheiro em centavos, quantidade em centésimos**, sempre inteiros.
- **Numeração** por `UPSERT` em `sequencia_documento` dentro da transação de criação;
  o lock de linha do Postgres resolve a concorrência. Número nunca é reaproveitado.
- **Optimistic locking** via `ordem_servico.versao`; conflito responde HTTP 409 com aviso.
- **Expirado é derivado** da validade (`valido_ate`), nunca gravado.
- **Cliente e serviço são copiados na OS** (vínculo + snapshot): editar o cadastro
  depois não muda OS antigas.
- **Dashboard** conta só agendada/em andamento/concluída, por data de emissão;
  "a receber" e "orçamentos aguardando" são a situação atual, independente do mês.
- **`empresa_id` em todas as tabelas**, com a empresa 1 fixa por enquanto; as consultas
  já filtram por ela.
- **Busca sem acento** (`unaccent`): "joao" encontra "João".
- **Imagens**: JPEG, PNG e WebP; HEIC (iPhone) não é aceito. Miniatura de 480px com
  correção de orientação EXIF.
