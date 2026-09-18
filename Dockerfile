# Imagem da API. Build em duas etapas: compila com o SDK do Go e roda num
# Alpine mínimo, sem compilador, como usuário sem privilégios.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
RUN mkdir -p /dados/uploads && chown -R app:app /dados
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
