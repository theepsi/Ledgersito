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

The name/idempotency extension is awaiting real-database verification after
applying migration 2; see the setup instructions.

Run the domain and application tests without Docker:

```sh
go test -v ./internal/domain/... ./internal/application/...
```

- [Local setup](docs/SETUP.md)
- [Architecture](docs/DESIGN.md)
- [Feature specifications](docs/specs/README.md)
