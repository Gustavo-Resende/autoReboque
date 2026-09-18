# docs/

Documentação de engenharia do backend. O `CLAUDE.md` na raiz resume tudo o
que precisa estar em toda sessão; aqui fica o que é consultado sob demanda.

| Arquivo | O que é | Quando ler |
|---|---|---|
| [`decisoes.md`](decisoes.md) | decisões de arquitetura numeradas (D-001…), com contexto e consequências | antes de propor mudança estrutural; ao se perguntar "por que é assim?" |
| [`planos/2026-09-18-arquitetura-hexagonal.md`](planos/2026-09-18-arquitetura-hexagonal.md) | plano de implementação por fases, com entregas, aceite e commits esperados | no início de toda sessão de implementação; marcar o progresso nele |

Convenções:

- Um plano por iniciativa grande, em `planos/AAAA-MM-DD-nome.md`, com seção
  "Diário" no fim para registrar desvios.
- Decisões nunca são editadas depois de aceitas: cria-se uma nova que
  substitui a antiga.
- Contratos de código (assinaturas de `pkg/`, `shared`, `cqrs`, `httpx`)
  vivem em `.claude/skills/*/references/` — são a especificação que as
  skills seguem. Mudou o código, muda o `references/` no mesmo commit.
