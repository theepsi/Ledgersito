-- name: ReserveTransfer :execrows
INSERT INTO transfer_replays (key, source_account_id, destination_account_id, amount)
VALUES ($1, $2, $3, $4) ON CONFLICT (key) DO NOTHING;

-- name: GetTransferReplay :one
SELECT source_account_id, destination_account_id, amount, status_code, response_body
FROM transfer_replays WHERE key = $1;

-- name: FinishTransfer :exec
UPDATE transfer_replays SET status_code = 201, response_body = $2 WHERE key = $1;

-- name: LockTransferAccountsTable :exec
LOCK TABLE accounts IN ROW EXCLUSIVE MODE;

-- name: LedgerInitialized :one
SELECT EXISTS (SELECT 1 FROM ledger_initialization) AS initialized;

-- name: LockTransferAccounts :many
SELECT id, role, balance FROM accounts
WHERE id = sqlc.arg(source_id) OR id = sqlc.arg(destination_id)
ORDER BY id COLLATE "C" FOR UPDATE;

-- name: CreateTransferTransaction :exec
INSERT INTO ledger_transactions (id, reference) VALUES ($1, '');

-- name: AddTransferEntry :exec
INSERT INTO entries (transaction_id, account_id, amount) VALUES ($1, $2, $3);

-- name: SetTransferBalance :execrows
UPDATE accounts SET balance = $2 WHERE id = $1;

-- name: PostTransferTransaction :exec
UPDATE ledger_transactions SET posted = TRUE WHERE id = $1;
