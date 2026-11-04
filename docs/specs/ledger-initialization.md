# Ledger Initialization

## Status and purpose

Approved, implemented, and verified against PostgreSQL. The user confirmed
migration 3 and the integration/initialization checks pass.

Establish a fixed fictional supply once, with balanced opening history.
The reservoir is a learning scenario, not a requirement of all ledgers,
real gold backing, or a blockchain.

## Supply and roles

- Fix the initial supply at 1,000,000 Sitos: 100,000,000 sititos.
- Account roles are `ordinary`, `reservoir`, and `origin`. A role describes
  ledger behavior, not a user or authorization identity.
- Existing accounts become ordinary without changing IDs, names, or balances.
- Normal account creation always creates an ordinary account with zero
  balance. POST callers cannot choose a role.
- There is exactly one reservoir and one origin per initialized ledger.
- Ordinary accounts and the reservoir cannot have negative balances.
- The origin is a non-spendable bookkeeping counterpart. It cannot participate
  in ordinary transfers; its negative balance is not an ordinary overdraft.
- The reservoir represents undistributed supply. The opening initialization
  credits it once; subsequent transfers may distribute funds to ordinary
  accounts but cannot return funds to the reservoir. This distribution-only
  policy is defined in [Transfers](transfers.md), which is not yet implemented.

## Initialization operation

- Provide an explicit CLI command requiring a database connection. Do not
  initialize during migrations, API startup, or ordinary account creation.
- This version has no configurable supply or additional issuance operation.
- In one database transaction, establish the initialization record, system
  accounts, opening ledger transaction, its two entries, and cached balances.
- Record exactly these opening entries:

| Account role | Signed sititos |
|---|---:|
| origin | -100000000 |
| reservoir | 100000000 |

- Each system account has a stable ID and a human-readable name.
- The origin is `system-origin`, named `Supply origin`; the reservoir is
  `system-reservoir`, named `Reservoir`. The opening transaction is
  `system-opening`, with reference `initial supply`.
- Invoke with `go run ./cmd/initledger` and an explicit `DATABASE_URL`.
  The command has a 30-second operation deadline and exits nonzero on errors.
- Keep pre-existing ordinary accounts unchanged. A zero-balance account needs
  no opening entry.
- Initialization must not silently absorb unexplained nonzero balances or
  pre-existing conflicting ledger state. Report an error without modifying it.
- Return success only after commit. On a failure before commit, leave none of
  the initialization writes persisted.
- A lost connection during commit can leave the outcome unknown. A repeat
  invocation must safely determine the persisted state, not issue again.

## Repeated and concurrent invocation

- A repeat against the same valid initialized ledger reports
  `already initialized` and exits successfully without additional entries,
  accounts, or funds.
- Conflicting supply, inconsistent initialization records, or partial/corrupt
  initialization state cause an explicit error and nonzero exit.
- Do not automatically repair corruption or reset balances.
- Enforce single initialization at the database boundary, including competing
  processes. Do not rely only on a preliminary existence check or a Go mutex.
- Concurrent invocations against an uninitialized database produce exactly one
  committed initialization. Other successful invocations report already
  initialized after the winner commits.
- Do not require the reservoir to retain its opening balance on a later repeat:
  valid transfers will redistribute funds.

## History and invariants

- Entries are append-only. Later corrections use compensating transactions.
- Every ledger transaction's signed entries sum to zero.
- Each account's cached balance equals the sum of its entries; an account
  without entries has zero balance.
- Reservoir plus ordinary balances total 100,000,000 sititos.
- The origin balance remains -100,000,000 sititos; including it, all balances
  sum to zero.
- Subsequent migrations must preserve supply, identity, and recorded history.
- Resetting a disposable database creates a new ledger, not continuity with
  the deleted one. These guarantees do not protect against privileged manual
  tampering or database destruction.

## API extension

- GET includes ordinary and system accounts, using existing name-then-ID order.
- Add `role` to listed account objects and new POST creation responses.
- POST accepts only `name`, as before; an input role is an unknown field.
- Existing replay records must retain their exact successful status and body.
  Their bodies must not be rewritten merely to add the new role field.
- Consequently, a replay recorded before this extension may lack `role`;
  listing and newly recorded creation responses include it. Document this
  compatibility behavior alongside the API.

## Acceptance scenarios

- Empty ledger initialization produces two system accounts, one opening
  transaction, two balanced entries, and the agreed supply.
- Initialization preserves existing zero-balance ordinary accounts.
- Repeat initialization changes no persistent ledger state.
- Concurrent initialization produces a single opening transaction.
- Failure between initialization writes rolls everything back.
- Inconsistent or conflicting state is rejected without repair or extra funds.
- Existing account migration preserves identity, name, and balance and assigns
  the ordinary role.
- GET exposes roles and system accounts; POST cannot select a system role.
- Old stored successful responses replay byte-for-byte unchanged.

Use focused domain/application tests and opt-in real-PostgreSQL integration
tests. Schema migrations and initialization remain user-run operations.

## Database enforcement

- Migration 3 adds roles, transaction headers, signed entries, and a singleton
  initialization record. It creates schema only; it does not issue funds.
- Account IDs and roles cannot change. Non-origin balances cannot be negative;
  the origin balance is fixed after creation.
- Entry updates, deletes, and truncation are rejected by triggers.
- A transaction is assembled as unposted, then marked posted in the same
  database transaction. A deferred constraint trigger checks at commit that
  it is posted with at least two entries summing to zero.
- Posted transactions cannot be changed or deleted and cannot receive more
  entries. Entry insertion locks its transaction header to serialize against
  posting; origin entries are allowed only in the opening transaction.
- Initialization uses a PostgreSQL function within the adapter's transaction.
  A table lock serializes initializers and excludes account writes while
  validating state. Listings remain possible. Repeats check the opening
  records, every account's entry sum, transaction balance, and conserved supply.
- Migration 3 is forward-only: its Down operation fails instead of deleting
  ledger history. A future corrective schema change requires a new migration.
- These protections assume ordinary SQL operation, not a privileged operator
  disabling triggers or dropping tables.

## Non-goals

- Transfer endpoints, purchases, mining, replenishment, or variable issuance.
- Users, ownership, authentication, and authorization.
- A general accounting chart of accounts or debit/credit account classes.
- Automatic corruption recovery.
