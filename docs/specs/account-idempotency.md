# Account Creation Idempotency

## Status and purpose

Agreed behavior for safe account-creation retries. This extends
[Create Account](create-account.md), whose original implementation has no
idempotency support. No implementation of this extension exists yet.

The HTTP contract, key validation, timeouts, and nickname schema migration
still need separate agreement before end-to-end implementation.

## Request identity

- `POST /accounts` requires an `Idempotency-Key`.
- Keys are scoped to this operation and shared across callers because the
  project has no authenticated clients.
- Compare validated, normalized request data, not raw JSON bytes.
- The nickname is required: 3-30 ASCII letters, digits, or underscores,
  starting with a letter. Normalize ASCII letters to lowercase.
- `Dragon` and `dragon` identify the same creation payload. JSON whitespace
  and formatting do not change request identity.
- Nickname uniqueness is separate from operation identity. Two different keys
  cannot create two accounts with the same normalized nickname.
- Renaming is outside this scope.

## Successful replay

- The first successful operation creates one zero-balance SITO account and
  stores its successful response.
- Reusing the key with the same normalized payload returns the original
  successful status and response body without creating another account.
- Replay returns the stored creation response, not a reconstruction from the
  account's current state.
- Reusing a committed key with a different normalized payload is rejected;
  it never changes the original account or replay record.
- A simultaneous request with the same key waits within its request timeout
  for the active database transaction to resolve. If that transaction commits,
  compare against its stored payload and replay or reject accordingly.
- If the active transaction rolls back, it has not reserved the key. A waiting
  request can attempt creation with its own validated payload.
- Different keys with the same normalized nickname produce one successful
  creation and nickname conflicts for competing creations, not replays.

## Atomicity and failures

- Persist the account and successful replay record in one database
  transaction. They must commit together or neither is persisted.
- Validation errors and nickname conflicts are not stored for replay and do
  not reserve the attempted key.
- Failures before commit roll back account and replay writes.
- A timeout while waiting is not a successful creation or replay. It must not
  overwrite another request's record.
- A lost connection during commit can leave its outcome unknown to the caller.
  Do not claim that such an error proves rollback. Retry with the same key to
  recover a committed result or attempt creation if nothing committed.
- If commit succeeds but the HTTP response is lost, the committed replay
  record survives and a same-key retry returns the original result.
- Keys do not expire automatically in this scope.

## Acceptance scenarios

| Scenario | Expected outcome |
|---|---|
| Same key, `Dragon` then `dragon` | One account; original successful status and body replayed |
| Same key, equivalent JSON formatting | Replay rather than a new account |
| Same committed key, different normalized nickname | Key-reuse error; original records unchanged |
| Concurrent same-key, same-payload requests | Wait then replay; one account and one successful replay record |
| Concurrent same-key, different-payload requests | After one commits, the competing payload is rejected |
| Different keys, same normalized nickname | One account; remaining creations report nickname conflicts |
| Invalid nickname followed by valid input with the same key | Invalid request does not reserve the key |
| Nickname conflict | No new account or replay record for the attempted key |
| Failure after account insert but before commit | Neither the account nor replay record survives |
| Response lost after commit | Same-key retry returns the stored success |

Use focused application tests for coordination and real-PostgreSQL integration
tests for transactional atomicity and competing requests. HTTP tests must
eventually verify replay of the actual status and body.

## Non-goals

- Authentication, client-specific key scoping, or authorization.
- Key expiry, retention cleanup, and rate limiting.
- Replaying unsuccessful outcomes.
- Applying idempotency to transfers before their own spec.
- Listing accounts and nickname migration policy.
