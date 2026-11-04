-- name: CreateAccount :exec
INSERT INTO accounts (id, balance, name) VALUES ($1, $2, $3);

-- name: ListAccounts :many
SELECT id, name, balance FROM accounts ORDER BY name COLLATE "C", id COLLATE "C";

-- name: ReserveCreation :execrows
INSERT INTO account_creation_replays (key, request_name) VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING;

-- name: GetCreation :one
SELECT request_name, status_code, response_body FROM account_creation_replays WHERE key = $1;

-- name: FinishCreation :exec
UPDATE account_creation_replays SET status_code = $2, response_body = $3 WHERE key = $1;
