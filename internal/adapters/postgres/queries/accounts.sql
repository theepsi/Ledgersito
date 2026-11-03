-- name: CreateAccount :exec
INSERT INTO accounts (id, balance) VALUES ($1, $2);
