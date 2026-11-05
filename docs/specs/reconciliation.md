# Ledger Reconciliation

## Status and purpose

Proposed; implementation is not approved yet.

Independently verify persisted ledger correctness without changing account
balances, ledger history, replay records, initialization state, or schema. This
is a diagnostic read, not a repair or recovery operation.

## Interface and execution

- Provide an explicit `go run ./cmd/reconcile` CLI command. Do not reconcile
  during API startup or ordinary requests.
- Require `DATABASE_URL`; do not apply migrations or initialize the ledger.
- Run all checks in one PostgreSQL `REPEATABLE READ`, read-only transaction.
  Concurrent committed transfers may proceed; reconciliation reports the
  coherent database snapshot visible to its transaction, not a mixture of
  states from separate queries.
- Emit a human-readable report. Report every detected invariant finding in a
  run, in deterministic order, followed by a summary.
- Use these exit codes:
  - `0`: initialized ledger is consistent.
  - `1`: one or more invariant findings.
  - `2`: configuration, connection, query, transaction, or other operational
    failure. Do not report operational failures as ledger findings.
  - `3`: clean, not-yet-initialized ledger.
- On operational failure, emit a concise error to stderr, do not claim the
  ledger is consistent, and do not return a partial report as a successful
  reconciliation.

## Uninitialized ledger

A ledger is cleanly uninitialized when it has no initialization record,
transactions, or entries; all existing accounts are ordinary with zero
balances; and neither reserved system account ID is present. Existing ordinary
accounts are allowed. Report that the ledger has not been initialized and exit
with `3`; do not treat this state as corruption or initialize it.

If the initialization record is absent but ledger history, a nonzero balance,
a system-role account, or a reserved system account ID exists, report the
applicable invariant findings and exit with `1`. Do not mask partial or
conflicting state as a clean uninitialized ledger.

## Checks for an initialized ledger

Check all of the following against the same snapshot:

- **Initialization metadata:** exactly one initialization record exists; its
  supply is `100000000` sititos and its opening transaction ID is
  `system-opening`.
- **System accounts:** exactly one origin and one reservoir exist. The origin
  is `system-origin`, named `Supply origin`, with role `origin`; the reservoir
  is `system-reservoir`, named `Reservoir`, with role `reservoir`. Other
  accounts have role `ordinary`.
- **Opening history:** `system-opening` is posted with reference `initial
  supply` and has exactly two entries: `-100000000` for `system-origin` and
  `+100000000` for `system-reservoir`. No other transaction has an origin
  entry.
- **Transaction integrity:** every ledger transaction is posted, has at least
  two entries, and the exact sum of its signed entries is zero.
- **Cached balances:** each account balance equals the exact sum of its
  entries; an account with no entries has balance zero.
- **Supply and balance rules:** the origin balance is exactly `-100000000`;
  ordinary and reservoir balances are nonnegative and total `100000000`; all
  account balances together total zero.
- **Reservoir policy:** after the opening transaction, no reservoir entry is
  positive. This confirms that the reservoir only distributes funds.
- **Referential integrity:** every entry refers to an existing account and
  transaction, and the initialization record refers to the opening
  transaction.

Use exact arithmetic for aggregate sums; PostgreSQL `SUM(bigint)` numeric
results must not be narrowed before comparison.

Report all findings rather than stopping at the first mismatch. Each finding
should identify a stable invariant code, a concise description, and the
affected account or transaction ID when applicable. Do not expose credentials
or raw database error details. A consistent initialized ledger with no
findings exits `0`; any findings exit `1`.

## Acceptance scenarios

- A clean uninitialized database, with or without existing zero-balance
  ordinary accounts, is reported as not initialized and is not modified.
- A newly initialized ledger reconciles successfully.
- Reservoir-to-ordinary and ordinary-to-ordinary transfers preserve successful
  reconciliation. Reconcile a database before and after transfers, and while
  transfers are running; no mixed-snapshot false findings are produced.
- Accounts without entries reconcile with zero balance.
- Isolated test schemas with a balance mismatch, unposted or unbalanced
  transaction, invalid system identity or metadata, incorrect opening
  history, negative non-origin balance, incorrect supply, positive post-opening
  reservoir entry, or dangling reference produce findings and exit `1`.
- A missing initialization record with no other ledger state is reported as
  uninitialized; a missing record with partial ledger state is reported as
  findings.
- A connection or query failure exits `2` and is not presented as a
  consistency result.
- Reconciliation performs no writes. Tests verify that account rows, entries,
  transactions, initialization metadata, and replay records are unchanged.

Use focused command/application tests and opt-in PostgreSQL integration tests.
Any deliberately inconsistent state must be confined to isolated test schemas.
Standard integration tests must not prepare or modify a database.

## Non-goals

- Repairing balances, replay records, transactions, metadata, or schema.
- Applying migrations, initializing or resetting a ledger, or changing
  initialization behavior.
- An HTTP endpoint, automatic background reconciliation, or alerting.
- A general-purpose accounting audit or support for multiple currencies.
