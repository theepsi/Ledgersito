# Ledgersito

**A learning project, not a production system.**

Ledgersito explores Go and double-entry ledger correctness. It is not intended
for commercial use, real-world deployments, or real money.
There is no support or maintenance commitment; forks are welcome.
Accounts are ledger accounts, not users or login identities. The
`name` field is a non-unique human-readable label for learning and recognizing
accounts; generated IDs identify accounts.
The project has no authentication or authorization and has not had a security
review. It is provided as is, without warranty; see [LICENSE](LICENSE).

## Current state

The project foundation, Money and Account domain types, and Create Account
application use case are in place with unit tests. PostgreSQL account storage,
its migrations, and opt-in integration tests are also provided. The API exposes
`POST /accounts` with idempotent creation and `GET /accounts` ordered by name
then ID. Names are non-unique labels; balances use integer sititos.
`POST /transfers` moves existing funds atomically with balanced entries and
idempotent retries. Features are specified and developed one at a time using
spec-first TDD.

Names and account-creation idempotency have passed database and manual API
checks. One-time fixed-supply initialization has also passed its PostgreSQL
integration checks.
Transfers have passed unit and real-PostgreSQL integration checks, including
concurrency, replay, rollback, and timeout scenarios.

## Fixed-supply learning scenario

The explicit `go run ./cmd/initledger` command establishes 1,000,000 fictional
Sitos (100,000,000 sititos) once. A reservoir receives the positive opening
entry and an origin bookkeeping account receives the equal negative entry.
Repeated initialization validates the ledger and never adds funds.

Migration 3 creates the schema but does not initialize it. The origin is not
spendable; ordinary accounts and the reservoir cannot overdraw. GET lists all
account roles; new POST results include `"role":"ordinary"`. Old successful
responses replay exactly as stored and may not include a role.

The reservoir is a simulation policy, not gold backing, a blockchain, or a
requirement of double-entry ledgers. The reservoir only distributes to ordinary
accounts; funds cannot return to it. Ordinary accounts can transfer among
themselves, and no transfer may involve the origin.
Apply migrations explicitly before running the command; migration 3 refuses
rollback rather than destroy ledger history.

## Transfers

After applying migration 4 and explicitly initializing the ledger,
`POST /transfers` accepts an `Idempotency-Key` and:

```json
{
  "source_account_id": "system-reservoir",
  "destination_account_id": "an-existing-ordinary-account-id",
  "amount_sititos": 250
}
```

Success returns 201 with the transfer ID, both account IDs, and
`"amount":{"sititos":250,"currency":"SITO"}`. Use positive integer sititos;
the source must have sufficient funds. Entries, balances, and the successful
response commit together. Same-key retries return the exact original response
without moving funds again. On timeout, retry with the same key: the outcome
may already be committed. Transfer keys are separate from account-creation
keys, and failed attempts do not reserve them.

See the [transfer spec](docs/specs/transfers.md) for validation, errors, and
concurrency guarantees. Migration 4 also refuses rollback to preserve replay
history.

## Reconciliation

With the existing migrations applied and `DATABASE_URL` configured,
`go run ./cmd/reconcile` independently checks balances, balanced posted history,
fixed supply, and system-account rules in one read-only consistent snapshot.
It does not initialize, migrate, or repair the ledger, and concurrent transfers
can continue.

The text report lists every detected invariant finding. Process exit codes are
`0` consistent, `1` findings, `2` operational failure, and `3` cleanly
uninitialized. Build and run the executable when exact codes are needed:
`go run` may itself exit `1` for any nonzero program exit.
See the [reconciliation spec](docs/specs/reconciliation.md). The feature has
passed its real-PostgreSQL acceptance checks.

Run the domain and application tests without Docker:

```sh
go test -v ./internal/domain/... ./internal/application/...
```

- [Local setup](docs/SETUP.md)
- [Architecture](docs/DESIGN.md)
- [Feature specifications](docs/specs/README.md)
- [Learning roadmap and session handoff](docs/ROADMAP.md)
