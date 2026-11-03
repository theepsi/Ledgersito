# Create Account

## Purpose and scope

Coordinate account creation at the application layer, using the existing
[Accounts](accounts.md) domain rules. Learn how a use case depends on small
interfaces without depending on a database, HTTP, or an ID-generation library.

## Inputs and boundaries

- The caller supplies no account ID, currency, or opening balance.
- The use case depends on an ID generator and account creation storage.
  Define these interfaces in the application layer that consumes them.
- The generator supplies an ID or an error. Its concrete format and generation
  algorithm are outside this spec.
- Storage accepts a domain account and reports whether creation succeeded.
  Its operation is create-only: an existing account must never be overwritten.
- Production storage must enforce ID uniqueness atomically, not rely on a
  separate existence check. Its implementation belongs to a later spec.

## Successful creation

For each invocation:

1. Request an ID from the generator exactly once.
2. Construct the account using the existing domain constructor.
3. Ask storage to create that account exactly once.
4. Return the account only after storage reports success.

The returned account has the exact generated ID and zero sititos in SITO.
The account sent to storage and the returned account have the same ID and
balance. The use case does not add metadata or change the domain rules.

## Failure behavior

- A generator error stops processing before domain construction or storage.
- An invalid generated ID produces the domain validation error and no storage
  call. Do not repair, trim, or replace the ID.
- A storage error is returned as a failure, never a successful account result.
- If the ID already exists, storage reports a conflict and leaves the existing
  account unchanged. The use case reports that error.
- Do not retry generation or storage, including on an ID conflict.
- Errors must remain identifiable by callers using Go's `errors.Is`, even if
  wrapped with contextual information.
- On any error, the returned account must be ignored; its contents are not
  part of the contract.
- Do not panic for these dependency or validation failures.

An error from storage does not necessarily mean nothing was persisted. For
example, a database might commit before its connection fails. This use case
must not claim rollback, attempt compensation, or automatically retry.
Request idempotency and recovery from uncertain outcomes need a later spec.

## Acceptance examples

| Scenario | Expected outcome |
|---|---|
| Generator returns `account-1`; storage succeeds | One generation, one save of `account-1` with zero balance, then return that account |
| Generator fails | Identifiable generator error; no storage call |
| Generator returns an empty ID without an error | Identifiable domain validation error; no storage call |
| Generator returns an ID containing whitespace | Identifiable domain validation error; no storage call |
| Storage fails | Identifiable storage error; no successful result or retry |
| Storage reports an existing ID | Identifiable conflict error; no overwrite, replacement ID, or retry |
| Two invocations receive distinct valid IDs | Each independently generates and saves its own zero-balance account |

Use focused application unit tests with small generator and storage test
doubles. Verify the account supplied to storage, call counts, and that saving
occurs before returning success. A create-only storage double can demonstrate
conflict handling, but does not prove a future database adapter enforces
uniqueness; that requires integration tests under its own spec.

These tests need neither Docker nor PostgreSQL.

## Non-goals

- Concrete ID generation, UUID dependencies, or globally proving uniqueness.
- PostgreSQL adapters, migrations, SQL queries, or repository reads.
- HTTP endpoints, status codes, and request idempotency.
- Automatic retries, compensation, and recovery after uncertain saves.
- Funding, balance changes, ledger entries, or transfers.
- Account names; these are planned for a later spec.
- Broader request cancellation and timeout policies.
