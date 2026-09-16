-- Banco separado para os testes de integração (go test com TEST_DATABASE_URL).
-- Os testes fazem TRUNCATE nas tabelas, por isso nunca apontam para o banco de uso real.
CREATE DATABASE autosocorro_test OWNER autosocorro;
