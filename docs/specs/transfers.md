# Transfers

## Status and purpose

Core rules, retry behavior, and HTTP request/success shapes approved.
Validation details and the HTTP error contract remain undecided. Not implemented.

Move existing fictional Sitos between accounts without changing total supply,
while learning balanced entries, atomic transactions, and concurrency safety.

## Approved core rules

- Require successful explicit ledger initialization before accepting transfers.
  An uninitialized ledger returns an explicit error; a transfer never initializes
  it automatically.
- Transfer a positive whole number of sititos representable as signed int64.
- Source and destination must be different existing accounts.
- Ordinary accounts and the reservoir can send and receive.
- The origin cannot be either endpoint.
- The source must have sufficient funds; neither resulting balance may overflow
  int64. Reject an invalid transfer without changing balances or history.
- Do not charge fees, convert currencies, or automatically fund an account.

## Atomicity and concurrency

- Record one negative source entry and one equal positive destination entry
  in a single posted ledger transaction.
- Persist both entries and both cached-balance updates atomically: commit
  everything or persist nothing.
- Preserve the fixed supply and all existing immutable ledger history.
- Concurrent transfers must not spend the same available funds twice.

For example, transferring 250 sititos from a reservoir holding 100,000,000
sititos to an ordinary account holding zero records entries of -250 and +250.
The resulting balances are 99,999,750 and 250 sititos.

## Approved retry behavior

- Require an Idempotency-Key scoped to transfers, separate from account creation.
  Keys are shared across callers because there is no authentication.
- Compare source ID, destination ID, and integer sititos, not raw JSON bytes.
- A committed same-key, same-payload retry returns the original successful
  status and body without transferring again, regardless of later balances.
- Reusing a committed key with different transfer details is rejected.
- Concurrent same-key requests wait within the operation timeout for the active
  transaction. After a commit, replay or reject according to its payload.
  After a rollback, the key remains available.
- Persist only successful responses, atomically with entries and balances.
  Failed requests do not reserve keys; keys do not expire automatically.
- A request rejected for insufficient funds may succeed on a later same-key
  retry after funding. Failed outcomes are not replayed.
- A timeout or connection loss during commit may leave the outcome unknown.
  Retry with the same key to recover a committed result or attempt the transfer
  if nothing committed. Do not claim that a timeout proves rollback.

## Approved HTTP request and success

- Provide POST /transfers with an Idempotency-Key header.
- Request fields are source_account_id, destination_account_id, and
  amount_sititos. The amount is an integer number of sititos; SITO is implicit.
- Return 201 Created with a generated transfer ID, both account IDs, and an
  amount object containing integer sititos and currency SITO.
- Successful retries return the exact original status and body.
- Do not include balances in the response: they may change after the transfer.
  GET /accounts provides current balances.
- No human-readable reference or transfer-listing endpoint in this version.

Example request:

```json
{
  "source_account_id": "system-reservoir",
  "destination_account_id": "example-account",
  "amount_sititos": 250
}
```

Example successful response:

```json
{
  "id": "generated-transfer-id",
  "source_account_id": "system-reservoir",
  "destination_account_id": "example-account",
  "amount": {"sititos": 250, "currency": "SITO"}
}
```

## Decisions still required

- Strict input validation, routing details, limits, and HTTP error contract.
- Observable behavior when multiple validation or business errors apply.
- Focused acceptance scenarios and database failure/concurrency tests.

## Non-goals

- Additional issuance, automatic initialization, or supply changes.
- Users, ownership, authentication, and authorization.
- Fees, currency conversion, and multi-recipient transfers.
- Human-readable transfer references and transfer-listing endpoints.
