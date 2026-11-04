-- +goose Up

-- Rust answers A2S only on its own query port (28017 next to game port 28015
-- and RCON port 28016), but the server form used to save query_port equal to
-- server_port for Rust. The query handler reads the saved value, and the Rust
-- start command now passes it to the server as well, so existing servers are
-- moved to the offset the form picks now (query_port_offset = 2). A query port
-- that differs from the game port was set deliberately and is kept.
UPDATE servers
SET query_port = server_port + 2
WHERE (query_port IS NULL OR query_port = server_port)
  AND server_port <= 65533
  AND game_id IN (SELECT code FROM games WHERE LOWER(engine) = 'rust');

-- +goose Down

-- The previous value never worked for Rust, so it is not restored.
SELECT 1;
