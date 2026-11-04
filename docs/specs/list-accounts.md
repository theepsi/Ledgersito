# List Accounts

## Status and scope

Agreed listing behavior for `GET /accounts`. Implemented; real-database
verification is pending. See [HTTP rules](accounts-http.md) and
[Account Names](account-names.md) for the agreed migration policy.

Return all persisted accounts for this small educational ledger. Authentication,
ownership filtering, pagination, and account mutation are outside this scope.
Ledger accounts are not user registrations.

## Behavior

- Read-only operation; no idempotency key is required.
- Return `200 OK` with a JSON object containing an `accounts` array.
- Include every persisted account exactly once, sorted by name ascending and
  then ID ascending to break ties. Use byte-based ordering for both fields
  rather than locale-dependent ordering. Names are case-preserved and non-unique.
- Each item contains `id`, `name`, `role`, and a `balance` object with
  integer `sititos` and currency `SITO`.
- Return stored balances, not an assumed zero opening balance.
- With no accounts, return `{"accounts":[]}`, never null or a not-found error.
- Do not create accounts or change balances or idempotency records.
- A storage failure must not be disguised as a successful empty listing.

## Response example

```json
{
  "accounts": [
    {
      "id": "generated-id",
      "name": "Dragon's savings",
      "role": "ordinary",
      "balance": {
        "sititos": 0,
        "currency": "SITO"
      }
    }
  ]
}
```

## Acceptance scenarios

- An empty database produces status 200 and an empty array.
- Accounts inserted out of name order are returned in ascending order.
- Equal names are ordered by ID; all accounts with that name are included.
- Returned IDs, names, and balances match persisted values.
- A populated listing contains all accounts without duplicates.
- Listing succeeds without an idempotency key and does not modify stored data.
- After initialization, listing includes origin and reservoir alongside
  ordinary accounts, each with the corresponding role.
- Storage failure produces an explicit error, not status 200 with an empty array.

Use application tests for error propagation, PostgreSQL integration tests for
retrieval and ordering, and HTTP tests for status and response shape.
