-- Esquema da V2.
--
-- Convenções:
--   * texto opcional é NOT NULL DEFAULT '' (vazio = não preenchido);
--   * dinheiro em bigint de centavos, quantidades em bigint de centésimos;
--   * enums como text + CHECK, mais fáceis de evoluir que CREATE TYPE;
--   * toda tabela de negócio tem empresa_id: hoje só existe a empresa 1,
--     mas o produto é pensado para servir mais de uma guincheira.

-- unaccent faz "joao" encontrar "João" nas buscas. Vem com o Postgres (contrib).
CREATE EXTENSION IF NOT EXISTS unaccent;

-- ---------------------------------------------------------------------------
-- Empresa (a guincheira dona dos dados). Aparece no cabeçalho dos documentos.
-- ---------------------------------------------------------------------------
CREATE TABLE empresa (
    id                        bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nome_fantasia             text        NOT NULL DEFAULT '',   -- como aparece no sistema
    razao_social              text        NOT NULL DEFAULT '',
    cnpj                      text        NOT NULL DEFAULT '',
    endereco                  text        NOT NULL DEFAULT '',
    cidade                    text        NOT NULL DEFAULT '',
    telefone                  text        NOT NULL DEFAULT '',
    whatsapp                  text        NOT NULL DEFAULT '',
    email                     text        NOT NULL DEFAULT '',
    logo_chave                text        NOT NULL DEFAULT '',   -- chave no Storage; '' = sem logo
    cor_primaria              text        NOT NULL DEFAULT '#F4511E',
    orcamento_validade_dias   smallint    NOT NULL DEFAULT 3 CHECK (orcamento_validade_dias > 0),
    orcamento_condicoes       text        NOT NULL DEFAULT '',   -- texto impresso no rodapé do orçamento
    atualizada_em             timestamptz NOT NULL DEFAULT now()
);
INSERT INTO empresa (nome_fantasia, razao_social, cnpj, endereco, cidade, telefone, whatsapp, email, orcamento_condicoes) VALUES (
    'Auto Socorro Trevo',
    'AUTO SOCORRO TREVO LTDA',
    '00.000.000/0001-00',
    'Rua Exemplo, 123 - Centro',
    'Cidade/BA',
    '(73) 99986-0359',
    '(73) 99986-0359',
    'contato@exemplo.com.br',
    'Valores sujeitos a alteração após o prazo de validade. Pedágios e taxas de pátio, quando houver, são cobrados à parte.'
);

-- ---------------------------------------------------------------------------
-- Clientes. O cliente 1 é o "Serviço particular": usado quando não há dados
-- de quem contratou (sistema = true; não se edita nem se desativa).
-- ---------------------------------------------------------------------------
CREATE TABLE cliente (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    empresa_id    bigint      NOT NULL DEFAULT 1 REFERENCES empresa (id),
    tipo          text        NOT NULL DEFAULT 'PF' CHECK (tipo IN ('PF', 'PJ')),
    nome          text        NOT NULL,
    documento     text        NOT NULL DEFAULT '',   -- CPF ou CNPJ, como digitado
    telefone      text        NOT NULL DEFAULT '',
    email         text        NOT NULL DEFAULT '',
    endereco      text        NOT NULL DEFAULT '',
    cidade        text        NOT NULL DEFAULT '',
    categoria     text        NOT NULL DEFAULT 'PARTICULAR'
        CHECK (categoria IN ('PARTICULAR', 'OFICINA', 'LOCADORA', 'CONCESSIONARIA', 'SEGURADORA', 'ORGAO_PUBLICO', 'OUTRO')),
    observacoes   text        NOT NULL DEFAULT '',
    ativo         boolean     NOT NULL DEFAULT true,
    sistema       boolean     NOT NULL DEFAULT false,
    criado_em     timestamptz NOT NULL DEFAULT now(),
    atualizado_em timestamptz NOT NULL DEFAULT now()
);
-- Documento, quando informado, não se repete dentro da empresa.
CREATE UNIQUE INDEX cliente_documento_unico ON cliente (empresa_id, documento) WHERE documento <> '';
CREATE INDEX cliente_nome_idx ON cliente (empresa_id, lower(nome));
INSERT INTO cliente (nome, sistema, observacoes) VALUES
    ('Serviço particular', true, 'Cliente padrão para atendimentos sem cadastro.');

-- ---------------------------------------------------------------------------
-- Catálogo de serviços: o que a empresa cobra e quanto.
-- ---------------------------------------------------------------------------
CREATE TABLE servico (
    id                 bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    empresa_id         bigint      NOT NULL DEFAULT 1 REFERENCES empresa (id),
    nome               text        NOT NULL,
    unidade            text        NOT NULL DEFAULT 'UN' CHECK (unidade IN ('UN', 'KM', 'HORA', 'DIARIA')),
    preco_centavos     bigint      NOT NULL DEFAULT 0 CHECK (preco_centavos >= 0),
    quantidade_do_km   boolean     NOT NULL DEFAULT false,   -- a quantidade vem do km do trajeto
    ativo              boolean     NOT NULL DEFAULT true,
    ordem              smallint    NOT NULL DEFAULT 0,
    criado_em          timestamptz NOT NULL DEFAULT now(),
    atualizado_em      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX servico_empresa_idx ON servico (empresa_id, ativo, ordem);
INSERT INTO servico (nome, unidade, preco_centavos, quantidade_do_km, ordem) VALUES
    ('Saída de base',        'UN',     15000, false, 1),
    ('Quilometragem rodada', 'KM',       450, true,  2),
    ('Hora parada / espera', 'HORA',    8000, false, 3),
    ('Hora trabalhada',      'HORA',   12000, false, 4),
    ('Pedágio',              'UN',         0, false, 5),
    ('Adicional noturno',    'UN',     10000, false, 6),
    ('Diária de pátio',      'DIARIA',  5000, false, 7);

-- ---------------------------------------------------------------------------
-- Motoristas: só o nome, para não escrever "Carlos" de três jeitos.
-- ---------------------------------------------------------------------------
CREATE TABLE motorista (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    empresa_id  bigint      NOT NULL DEFAULT 1 REFERENCES empresa (id),
    nome        text        NOT NULL,
    telefone    text        NOT NULL DEFAULT '',
    ativo       boolean     NOT NULL DEFAULT true,
    criado_em   timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Contador de numeração por empresa, modelo de documento e ano.
-- O UPSERT nesta linha, dentro da transação que cria a OS, serializa dois
-- usuários criando ao mesmo tempo: o segundo espera o lock da linha.
-- ---------------------------------------------------------------------------
CREATE TABLE sequencia_documento (
    empresa_id      bigint      NOT NULL REFERENCES empresa (id),
    tipo            text        NOT NULL,   -- 'OS' (futuro: 'RECIBO', ...)
    ano             smallint    NOT NULL,
    ultimo_numero   integer     NOT NULL,
    PRIMARY KEY (empresa_id, tipo, ano)
);

-- ---------------------------------------------------------------------------
-- Ordem de serviço. Nasce como orçamento e é o mesmo registro (mesmo número)
-- durante toda a vida do serviço. A fase (orçamento ou OS) vem do status.
-- ---------------------------------------------------------------------------
CREATE TABLE ordem_servico (
    id                    bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    empresa_id            bigint      NOT NULL DEFAULT 1 REFERENCES empresa (id),
    numero                integer     NOT NULL,
    ano                   smallint    NOT NULL,
    versao                integer     NOT NULL DEFAULT 1,            -- optimistic locking
    origem_atendimento    text        NOT NULL DEFAULT 'PARTICULAR',  -- gancho p/ seguradoras
    status                text        NOT NULL DEFAULT 'ORCAMENTO_ABERTO' CHECK (status IN (
                              'ORCAMENTO_ABERTO', 'ORCAMENTO_ENVIADO', 'ORCAMENTO_RECUSADO',
                              'AGENDADA', 'EM_ANDAMENTO', 'CONCLUIDA', 'CANCELADA')),
    status_pagamento      text        NOT NULL DEFAULT 'PENDENTE'
        CHECK (status_pagamento IN ('PENDENTE', 'PARCIAL', 'PAGA')),

    data_emissao          date        NOT NULL DEFAULT CURRENT_DATE,  -- editável (serviços passados)
    valido_ate            date,                                       -- validade do orçamento
    agendada_para         timestamptz,                                -- quando status = AGENDADA
    acionado_em           timestamptz,                                -- hora em que o cliente chamou
    aprovada_em           timestamptz,                                -- quando virou OS
    concluida_em          timestamptz,

    -- Cliente: vínculo + cópia dos dados na emissão (editar o cadastro depois
    -- não reescreve OS antigas; a impressão mostra o que valia na época).
    cliente_id            bigint      NOT NULL REFERENCES cliente (id),
    cliente_nome          text        NOT NULL DEFAULT '',
    cliente_documento     text        NOT NULL DEFAULT '',
    cliente_telefone      text        NOT NULL DEFAULT '',
    solicitante_nome      text        NOT NULL DEFAULT '',   -- quem ligou, se não for o cliente
    solicitante_telefone  text        NOT NULL DEFAULT '',

    veiculo_categoria     text        NOT NULL DEFAULT ''
        CHECK (veiculo_categoria IN ('', 'LEVE', 'UTILITARIO', 'EXTRAPESADO')),   -- porte: define guincho e preço
    veiculo_modelo        text        NOT NULL DEFAULT '',
    veiculo_ano           text        NOT NULL DEFAULT '',   -- texto: "2018/2019" é comum
    veiculo_cor           text        NOT NULL DEFAULT '',
    veiculo_placa         text        NOT NULL DEFAULT '',
    veiculo_condicao      text        NOT NULL DEFAULT '',   -- "roda", "travado", "capotado"...

    trajeto_origem        text        NOT NULL DEFAULT '',
    trajeto_referencia    text        NOT NULL DEFAULT '',   -- ponto de referência do local
    trajeto_destino       text        NOT NULL DEFAULT '',
    trajeto_km            integer     NOT NULL DEFAULT 0 CHECK (trajeto_km >= 0),  -- 0 = não informado

    motorista_id          bigint      REFERENCES motorista (id) ON DELETE SET NULL,
    motorista_nome        text        NOT NULL DEFAULT '',   -- cópia, como no cliente
    guincho               text        NOT NULL DEFAULT '',   -- qual guincho fez o serviço
    recebido_por          text        NOT NULL DEFAULT '',   -- quem recebeu o veículo no destino
    observacoes           text        NOT NULL DEFAULT '',

    subtotal_centavos     bigint      NOT NULL DEFAULT 0,   -- soma dos itens
    desconto_centavos     bigint      NOT NULL DEFAULT 0 CHECK (desconto_centavos >= 0),
    total_centavos        bigint      NOT NULL DEFAULT 0,   -- subtotal - desconto
    valor_pago_centavos   bigint      NOT NULL DEFAULT 0 CHECK (valor_pago_centavos >= 0),
    forma_pagamento       text        NOT NULL DEFAULT ''
        CHECK (forma_pagamento IN ('', 'PIX', 'DINHEIRO', 'DEBITO', 'CREDITO', 'TRANSFERENCIA', 'FATURADO')),
    vencimento            date,                             -- quando faturado / a prazo

    criada_em             timestamptz NOT NULL DEFAULT now(),
    atualizada_em         timestamptz NOT NULL DEFAULT now(),

    UNIQUE (empresa_id, ano, numero)   -- número nunca se repete no ano
);
CREATE INDEX ordem_servico_status_idx  ON ordem_servico (empresa_id, status, ano DESC, numero DESC);
CREATE INDEX ordem_servico_emissao_idx ON ordem_servico (empresa_id, data_emissao);
CREATE INDEX ordem_servico_cliente_idx ON ordem_servico (cliente_id);

CREATE TABLE ordem_servico_item (
    id                      bigint    GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ordem_servico_id        bigint    NOT NULL REFERENCES ordem_servico (id) ON DELETE CASCADE,
    posicao                 smallint  NOT NULL,   -- ordem de exibição, a partir de 1
    servico_id              bigint    REFERENCES servico (id) ON DELETE SET NULL,  -- de onde veio (opcional)
    descricao               text      NOT NULL,
    unidade                 text      NOT NULL DEFAULT 'UN',
    quantidade_centesimos   bigint    NOT NULL CHECK (quantidade_centesimos >= 0),   -- 150 = 1,50
    valor_unitario_centavos bigint    NOT NULL CHECK (valor_unitario_centavos >= 0),
    subtotal_centavos       bigint    NOT NULL,
    UNIQUE (ordem_servico_id, posicao)
);

CREATE TABLE ordem_servico_imagem (
    id                bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ordem_servico_id  bigint      NOT NULL REFERENCES ordem_servico (id) ON DELETE CASCADE,
    posicao           smallint    NOT NULL,
    etapa             text        NOT NULL DEFAULT 'OUTRA' CHECK (etapa IN ('RETIRADA', 'ENTREGA', 'OUTRA')),
    chave_original    text        NOT NULL,   -- chave no Storage
    chave_miniatura   text        NOT NULL,
    nome_original     text        NOT NULL,
    content_type      text        NOT NULL,
    tamanho_bytes     bigint      NOT NULL,
    criada_em         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ordem_servico_imagem_os_idx ON ordem_servico_imagem (ordem_servico_id, posicao);

-- Sessões de login. Guardamos só o hash do token que vai no cookie.
CREATE TABLE sessao (
    token_hash   text        PRIMARY KEY,
    criada_em    timestamptz NOT NULL DEFAULT now(),
    expira_em    timestamptz NOT NULL
);
CREATE INDEX sessao_expira_idx ON sessao (expira_em);
