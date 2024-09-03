CREATE TABLE clientes (
  id SERIAL PRIMARY KEY,
  limite INT NOT NULL,
  saldo INT NOT NULL
);

CREATE UNLOGGED TABLE transacoes (
  id SERIAL PRIMARY KEY,
  cliente_id INT NOT NULL,
  valor INT NOT NULL,
  tipo varchar(1) NOT NULL,
  descricao varchar(10) NOT NULL,
  realizada_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (cliente_id) REFERENCES clientes(id) ON DELETE CASCADE
);

ALTER TABLE
  transacoes
SET
  (autovacuum_enabled = false);

CREATE INDEX idx_transactions ON transacoes (cliente_id asc);

CREATE OR REPLACE FUNCTION credito_cliente(cliente_id INT, valor INT, descricao VARCHAR(10)) RETURNS TABLE (
  limite INT,
  sucesso BOOLEAN,
  saldo_atual INT
)
LANGUAGE plpgsql
AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(cliente_id);

  INSERT INTO transacoes (cliente_id, valor, tipo, descricao) 
  VALUES (cliente_id, valor, 'c', descricao);

  RETURN QUERY
      UPDATE clientes c
      SET saldo = c.saldo + valor
      WHERE id = cliente_id
      RETURNING c.limite, TRUE, c.saldo;
END;
$$;

CREATE OR REPLACE FUNCTION debito_cliente(cliente_id INT, valor INT, descricao VARCHAR)RETURNS TABLE (
  limite_cliente INT,
  sucesso BOOLEAN,
  saldo_atual INT
)
LANGUAGE plpgsql
AS $$
DECLARE 
  saldo_atual_temp INT;
  limite_temp INT;
BEGIN
  PERFORM pg_advisory_xact_lock(cliente_id);

  SELECT saldo, limite 
  INTO saldo_atual_temp, limite_temp 
  FROM clientes 
  WHERE id = cliente_id;

  IF saldo_atual_temp - valor >= limite_temp * -1 THEN
    INSERT INTO transacoes (cliente_id, valor, tipo, descricao)
    VALUES (cliente_id, valor, 'd', descricao);

    RETURN QUERY
      UPDATE clientes c
      SET saldo = c.saldo - valor
      WHERE id = cliente_id
      RETURNING c.limite, TRUE, c.saldo;
  ELSE
    RETURN QUERY SELECT limite_temp, FALSE, saldo_atual_temp;
  END IF;
END;
$$;

INSERT INTO clientes (limite, saldo) VALUES (100000, 0);
INSERT INTO clientes (limite, saldo) VALUES (80000, 0);
INSERT INTO clientes (limite, saldo) VALUES (1000000, 0);
INSERT INTO clientes (limite, saldo) VALUES (10000000, 0);
INSERT INTO clientes (limite, saldo) VALUES (500000, 0);
