# Transaction History

## Status and purpose

Approved and implemented; the isolated PostgreSQL integration check passed.

Expose posted ledger transactions and their entries through a read-only API so
the ledger's history can be explored.

## HTTP behavior

- Provide exact `GET /transactions`; do not accept query parameters.
- Return `200 OK` with a JSON object containing a `transactions` array.
- Include every posted transaction, including the opening transaction.
- Each transaction contains its `id`, `reference`, and `entries`.
- Each entry contains its persisted `entry_id`, `account_id`, and signed
  `amount` object with integer `sititos` and currency `SITO`.
- Return empty arrays, not `null`, when there is no history.
- Do not require an idempotency key and do not write ledger data.
- Other methods return `405` with `Allow: GET`; unknown paths use the existing
  JSON `404` behavior.
- Reject query parameters using the existing API's `invalid_request` error.
- Use the existing five-second request deadline and JSON error envelope.
  Timeouts return `503`; storage or encoding failures return `500`, never an
  empty successful result.

## Ordering and timestamps

- Order transactions by their smallest entry ID, ascending; order each
  transaction's entries by entry ID, ascending.
- This ordering is deterministic entry-ID order, not a wall-clock timeline.
- Entry IDs may be allocated before their transaction commits.
- Do not add timestamps in this version. The current schema does not record
  them, and introducing them would require a migration and explicit timestamp
  semantics. The API must not imply that entry IDs are timestamps.

## Acceptance scenarios

- Empty history returns `{"transactions":[]}`.
- An initialized ledger includes the opening transaction and its two signed
  entries.
- Transfers appear with both persisted, opposite-signed entries.
- Multiple transactions and entries are returned in deterministic order.
- Returned fields and amounts match persisted values.
- A storage failure is reported as an error, not an empty result.
- Listing history does not change accounts, entries, transactions, or other
  ledger state.

## Non-goals

- Account-specific filtering, pagination, transaction mutation, or timestamps.
- Analytics, reconciliation, or changes to transfer semantics.
