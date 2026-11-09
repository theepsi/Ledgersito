-- name: ListTransactions :many
SELECT transaction_id, reference, entry_id, account_id, amount
FROM (
    SELECT t.id AS transaction_id, t.reference, e.id AS entry_id, e.account_id, e.amount,
           min(e.id) OVER (PARTITION BY t.id) AS first_entry_id
    FROM ledger_transactions t
    JOIN entries e ON e.transaction_id = t.id
    WHERE t.posted
) history
ORDER BY first_entry_id, entry_id;
