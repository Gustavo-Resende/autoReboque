---
name: criar-ferramenta
description: Cria ou estende um utilitário transversal em pkg/ do autoReboque (validação, erros, httpx, config, logger, clock, formatação) — genérico, só stdlib, sem regra de negócio, usável por qualquer camada. Use sempre que o pedido envolver "utilitário", "helper", "ferramenta", "extensão", "pkg/", "reaproveitar em várias camadas", "função genérica", "formatar/parsear", "regra de validação nova" ou quando perceber que a mesma função está sendo copiada em dois lugares.
---

# Criar ferramenta (`pkg/`)

`pkg/` é a caixa de ferramentas que **todas** as camadas podem usar
(`domain`, `application`, `adapters`, `cmd`). Justamente por isso ela não pode
saber nada do negócio: se `pkg/validar` importar `ordemservico`, o domínio não
consegue mais importar `validar` sem ciclo, e a ferramenta deixa de ser
ferramenta.

## Teste de pertencimento

Antes de criar algo em `pkg/`, responda:

1. **Serve para mais de uma camada ou mais de um módulo?** Se só o adapter
   HTTP usa, fica em `adapters/http`. Se só a OS usa, fica em
   `domain/ordemservico`.
2. **Funciona sem saber o que é OS, cliente ou empresa?** Se precisa de um
   tipo de negócio, não é ferramenta.
3. **Depende só da stdlib?** `pkg/` não importa `pgx`, `chi` nem nada de
   `internal/`. (Exceção única: `pkg/httpx` conhece `chi` para ler path
   params — é a fronteira HTTP; ainda assim não conhece `internal/`.)

Se alguma resposta é "não", o lugar é outro.

## Pacotes existentes (não duplicar)

| Pacote | Responsabilidade |
|---|---|
| `pkg/erros` | tipo `Erro` (Kind, Codigo, Mensagem, Campos), construtores, `KindDe`, `Juntar` |
| `pkg/validar` | regras de forma para entrada (obrigatório, tamanho, formato BR: telefone, placa, CPF/CNPJ) |
| `pkg/httpx` | decode/encode JSON, `EscreverErro`, params, sessão no ctx |
| `pkg/config` | leitura de variáveis de ambiente e `.env` |
| `pkg/logger` | construção do `*slog.Logger` (texto em dev, JSON em prod) |
| `pkg/relogio` | `Clock` real (`Agora()`), `Fixo(t)` para testes |

Ao adicionar uma **regra de validação** nova (ex.: `validar.CNPJ`), ela entra
em `pkg/validar` com teste de tabela cobrindo válidos, inválidos e vazio.

## Como escrever

- **Funções puras** com entrada e saída claras. Estado global zero. Se
  precisa de configuração, recebe por parâmetro ou por struct construída em
  `cmd/api`.
- **Erros no padrão** `pkg/erros`: uma ferramenta que valida devolve
  `erros.Campo(campo, msg)`; uma que falha por I/O devolve `erros.Interno`.
  Nunca `errors.New` solto, para o adapter HTTP conseguir mapear.
- **Nome curto no pacote, verbo na função**: `validar.Placa`,
  `erros.Conflito`, `httpx.Escrever`. Evitar `util`, `helpers`, `common`:
  nomes assim viram depósito.
- **Generics só quando eliminam duplicação real** (`guard.Entre[N]`); não
  para parecer sofisticado.
- **Doc comment em toda função exportada** dizendo o que aceita e o que
  devolve em falha. É o contrato que as skills das outras camadas citam.
- **Sem dependência de tempo/aleatoriedade escondida**: se precisa de
  `time.Now()`, recebe um `Clock`.

## Exemplo: nova regra de validação

```go
// Placa aceita o padrão antigo (ABC1234) e o Mercosul (ABC1D23), com ou sem
// hífen, em qualquer caixa. Vazio é aceito: combine com Obrigatorio.
func Placa(campo, valor string) error {
	v := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(valor), "-", ""))
	if v == "" {
		return nil
	}
	if !placaRe.MatchString(v) {
		return erros.Campo(campo, "placa inválida")
	}
	return nil
}

var placaRe = regexp.MustCompile(`^[A-Z]{3}[0-9][A-Z0-9][0-9]{2}$`)
```

```go
func TestPlaca(t *testing.T) {
	casos := []struct{ in string; ok bool }{
		{"ABC1234", true}, {"abc-1234", true}, {"ABC1D23", true},
		{"", true}, {"AB12345", false}, {"ABCD123", false},
	}
	for _, c := range casos {
		err := validar.Placa("placa", c.in)
		if (err == nil) != c.ok {
			t.Errorf("Placa(%q): ok=%v, err=%v", c.in, c.ok, err)
		}
	}
}
```

## Checklist

- [ ] Responde "sim" às três perguntas do teste de pertencimento
- [ ] `go list -deps ./pkg/<pacote> | grep -E "internal/|pgx"` vazio
- [ ] Erros no padrão `pkg/erros`; nenhum `errors.New`/`fmt.Errorf` como erro final
- [ ] Doc comment em cada função exportada
- [ ] Teste de tabela cobrindo válidos, inválidos, vazio e limites
- [ ] Atualizada a tabela "Pacotes existentes" acima e, se for contrato usado por outra skill, o `references/` dela
- [ ] Commit: `feat(pkg): validar.Placa` (ver `/commits`)
