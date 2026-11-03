# Ledgersito

**A learning project, not a production system.**

Ledgersito explores Go and double-entry ledger correctness. It is not intended
for commercial use, real-world deployments, or real money.
There is no support or maintenance commitment; forks are welcome.
The project has no authentication or authorization and has not had a security
review. It is provided as is, without warranty; see [LICENSE](LICENSE).

## Current state

The project foundation, Money and Account domain types, and Create Account
application use case are in place with unit tests. There is no runnable API,
database schema, or ledger implementation yet. Features are specified and
developed one at a time using spec-first TDD.

Run the domain and application tests without Docker:

```sh
go test -v ./internal/domain/... ./internal/application/...
```

- [Local setup](docs/SETUP.md)
- [Architecture](docs/DESIGN.md)
- [Feature specifications](docs/specs/README.md)
