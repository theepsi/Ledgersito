-- name: InitializeLedger :one
SELECT initialize_ledger()::boolean AS created;
