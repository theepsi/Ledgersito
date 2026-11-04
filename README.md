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
There is no transfer ledger yet. Features are specified and developed one at
a time using spec-first TDD.

Names and account-creation idempotency have passed database and manual API
checks. One-time fixed-supply initialization has also passed its PostgreSQL
integration checks.

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
requirement of double-entry ledgers. Transfer behavior remains to be specified.
Apply migrations explicitly before running the command; migration 3 refuses
rollback rather than destroy ledger history.

Run the domain and application tests without Docker:

```sh
go test -v ./internal/domain/... ./internal/application/...
```

- [Local setup](docs/SETUP.md)
- [Architecture](docs/DESIGN.md)
- [Feature specifications](docs/specs/README.md)
