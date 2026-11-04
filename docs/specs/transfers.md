# Transfers

## Status and purpose

Approved, including core rules, retry behavior, HTTP contract, input validation,
error precedence, and acceptance scenarios. Not implemented.

Move existing fictional Sitos between accounts without changing total supply,
while learning balanced entries, atomic transactions, and concurrency safety.

## Approved core rules

- Require successful explicit ledger initialization before accepting transfers.
  An uninitialized ledger returns an explicit error; a transfer never initializes
  it automatically.
- Transfer a positive whole number of sititos representable as signed int64.
- Source and destination must be different existing accounts.
- Ordinary accounts can send and receive. The reservoir can only send to
  ordinary accounts; it cannot receive transfers.
- The reservoir represents supply not yet distributed. Initialization creates
  the supply; reservoir transfers put it into circulation. Once distributed,
  funds circulate among ordinary accounts without returning to the reservoir.
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

## Approved validation, routing, and limits

- Require exactly the three request fields, using case-sensitive field names.
  Reject missing, unknown, duplicate, or null fields, malformed JSON,
  non-object bodies, and trailing data. Reject malformed UTF-8 and unpaired
  surrogate escapes, as in the account API.
- Account IDs must be nonempty strings without Unicode whitespace. Preserve
  accepted IDs exactly, without trimming or case conversion, following
  existing account identity rules.
- amount_sititos must use JSON integer notation from 1 through
  9,223,372,036,854,775,807 inclusive. Reject strings, fractions, and exponent
  notation, including mathematically integral forms such as 250.0 or 2.5e2.
  Representability does not imply sufficient funds.
- Reject identical source and destination IDs; do not treat them as a no-op.
- Reuse the account API's Idempotency-Key validation: exactly one value,
  1-128 ASCII letters, digits, hyphens, or underscores, case-sensitive and
  untrimmed.
- Require application/json, optionally with charset=utf-8 (case-insensitive);
  reject other media types and parameters.
- Limit the body to 4096 bytes including whitespace; exactly 4096 is permitted
  if otherwise valid.
- Use a five-second application/database operation timeout, including key and
  account-lock waits. Propagate cancellation to database work.
- Match exactly /transfers without trailing-slash or encoded-path aliases.
  Reject query strings, including a bare question mark. Do not redirect.
- Support only POST; other methods, including GET and HEAD, return 405 with
  Allow: POST. Unknown paths retain the existing JSON 404 behavior.
- Validate the request before attempting a transfer or reserving its key.

## Approved errors and precedence

Use the existing JSON error envelope with a stable code and human-readable
message. Do not expose SQL, credentials, or internal database errors.

| Condition | HTTP status | Error code |
|---|---|---|
| Invalid JSON, field types, missing/extra fields, or query string | 400 | invalid_request |
| Empty or whitespace-containing account ID | 400 | invalid_account_id |
| Integer amount zero, negative, or outside int64 | 400 | invalid_amount |
| Identical source and destination | 400 | same_account |
| Missing key | 400 | missing_idempotency_key |
| Empty, invalid, or repeated key | 400 | invalid_idempotency_key |
| Body exceeds 4096 bytes | 413 | request_too_large |
| Missing or unsupported Content-Type | 415 | unsupported_media_type |
| Ledger not initialized | 409 | ledger_not_initialized |
| Either account does not exist | 404 | account_not_found |
| Origin used as either endpoint, or reservoir used as destination | 422 | account_not_transferable |
| Insufficient source funds | 422 | insufficient_funds |
| Resulting balance would overflow | 422 | balance_overflow |
| Committed key reused with different details | 422 | idempotency_key_reused |
| Operation deadline exceeded | 503 | request_timeout |
| Unexpected internal failure | 500 | internal_error |

Fractions, exponent notation, and quoted amounts are invalid_request.
Integer-shaped amounts outside the allowed range are invalid_amount.

For valid input, resolve a committed same-key replay or payload conflict before
checking mutable business state. Otherwise require initialization, require both
accounts, reject origin participation or a reservoir destination, check source
funds, then check resulting balances, in that order. A successful replay must not fail because funds have
subsequently moved.

Precedence among multiple malformed-input errors is unspecified. Each isolated
condition has the code above; clients must correct all invalid input.

## Approved acceptance scenarios

- Reservoir-to-ordinary and ordinary-to-ordinary transfers succeed, including
  spending the entire source balance. Reservoir destinations and origin
  involvement fail without writes.
- Each success creates one posted transaction with exactly two opposite entries
  and atomically updates both balances. Balances match history; supply remains
  unchanged and existing history remains immutable.
- Invalid input, missing initialization/accounts, prohibited roles, and
  insufficient funds return the agreed errors without changing balances,
  history, or replay records.
- Same-key retries return the exact original successful status/body, including
  after later transfers. Different details conflict. Account-creation keys and
  transfer keys do not collide.
- An insufficient-funds attempt can succeed with the same key after funding.
  Concurrent same-key requests create only one transfer; rollback leaves its
  key available.
- With 100 sititos available, two concurrent distinct-key transfers of 80
  produce exactly one success and one insufficient_funds error.
- Concurrent opposite-direction transfers between funded ordinary accounts
  complete without an account-lock-order deadlock. Funding and amounts in this
  scenario permit both to succeed.
- Injected failures after partial writes roll back all changes. A simulated
  lost response after commit is recovered by same-key replay without moving
  funds twice.
- Lock-wait deadlines return request_timeout when a response is still possible.
  A same-key retry safely recovers the outcome.
- HTTP tests cover strict JSON, amount boundaries/notation, IDs, key validation,
  limits, routing, and error mapping.

Use focused unit tests and opt-in PostgreSQL integration tests. Database
preparation remains user-controlled.

Balance overflow is unreachable in a healthy fixed-supply ledger because total
supply is below int64 maximum. Test this defensive arithmetic rule at the
calculation boundary rather than creating corrupt database balances.

## Non-goals

- Additional issuance, automatic initialization, or supply changes.
- Users, ownership, authentication, and authorization.
- Fees, currency conversion, and multi-recipient transfers.
- Human-readable transfer references and transfer-listing endpoints.
