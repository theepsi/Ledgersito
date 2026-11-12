# Local setup

Run these commands yourself, one step at a time, from the repository root.
They are instructions, not setup performed by the assistant.

## 1. Check prerequisites

```sh
go version
docker --version
docker compose version
```

The module declares Go 1.27.0 or newer. PostgreSQL runs in Docker; no host
PostgreSQL installation is needed.

## 2. Install development tools when needed

Goose manages migrations. sqlc generates Go query code from SQL. Neither is
needed to run domain-only tests.

```sh
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

These commands download and build tools into `GOBIN`, or `$(go env GOPATH)/bin`
when `GOBIN` is unset. They do not add project dependencies to `go.mod`.
`@latest` selects the available release at installation time, not a pinned
version. Record and pin tool versions once the first working toolchain is
agreed.

With the default Go tool directory, add this to `~/.zshrc` if not already present:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then open a new terminal and verify:

```sh
goose -version
sqlc version
```

The lint configuration targets golangci-lint v2. Check official repository
availability first:

```sh
pacman -Ss golangci-lint
```

If it is available in your configured official repositories:

```sh
sudo pacman -S --needed golangci-lint
golangci-lint version
```

Do not install from the AUR. If the official repositories do not provide a
compatible release, agree on another official installation method first.

## 3. Add the database driver when ready

pgx is a project library, not a global executable:

```sh
go get github.com/jackc/pgx/v5
```

This downloads pgx and updates `go.mod` and `go.sum`. The PostgreSQL adapter
now imports it; no additional library is required for account storage tests.

Telemetry, UUID helpers, and testcontainers are deferred until their specs
demonstrate a need.

## 4. Configure and start the development database

```sh
cp .env.example .env
docker compose config --quiet
docker compose up -d --wait
docker compose ps
```

The first startup may download the PostgreSQL image and creates a named volume.
Docker must already be running. Do not enable services or change Docker group
membership without understanding their system and security implications.

The database port is bound only to the local machine. The credentials are
deliberately local-development-only; never reuse them elsewhere. `.env` is
ignored by Git. If port 5432 is occupied, change both `POSTGRES_PORT` and the
port in `DATABASE_URL` in `.env`.

Compose reads `.env` for interpolation; it does not export those variables into
your shell. Future application and migration commands will need an explicitly
provided `DATABASE_URL`; no automatic `.env` loading is implemented.

To check PostgreSQL readiness:

```sh
docker compose exec postgres pg_isready -U ledger -d ledger
```

To stop the development database while retaining its data:

```sh
docker compose down
```

Apply the versioned schema to the development database explicitly:

```sh
export DATABASE_URL='postgres://ledger:ledger@localhost:5432/ledger?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
```

Adjust the port if necessary. This creates the accounts table and goose's
migration-version table. Do not run migration rollback commands casually:
the first migration's rollback drops the accounts table and its data.

Regenerate typed query code after changing schema/query sources:

```sh
sqlc generate
```

Generated files in `internal/adapters/postgres/dbsql` belong in Git and should
not be manually edited. Generation was verified with sqlc v1.31.1; the installed
goose version during implementation was v3.29.0.

## 5. Run checks once Go code exists

```sh
go test ./...
go vet ./...
golangci-lint run
golangci-lint fmt
```

Equivalent Make targets are `make test`, `make vet`, `make lint`, and `make fmt`
if Make is installed. Formatting modifies source files.
Ordinary tests cover the domain, application, and adapter error mapping without
connecting to a database.

Go commands may download dependencies or a required toolchain. After preparing
dependencies yourself, use `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` before
Go validation commands to prevent those downloads.
Integration-test commands that start containers must be explicitly agreed
before execution; do not assume every test suite is free of setup side effects.

## 6. Prepare and run database integration tests

With the existing PostgreSQL container running, create a separate test database
once. This command fails if the database already exists; do not drop it to
silence that error.

```sh
docker compose exec postgres createdb -U ledger ledger_test
export TEST_DATABASE_URL='postgres://ledger:ledger@localhost:5432/ledger_test?sslmode=disable'
goose -dir migrations postgres "$TEST_DATABASE_URL" up
```

Adjust the URL port if needed. These are explicit user-run setup steps; tests
do not start containers, create databases, or apply migrations.

Then run:

```sh
go test -tags=integration -count=1 -v ./internal/adapters/postgres
```

Or use `make test-integration`. The integration selector requires
`TEST_DATABASE_URL`; missing configuration, connection failures, or unapplied
migrations fail rather than skip. Tests accept only the database name
`ledger_test` and verify the actual connected database before writes.
Only point this URL at a disposable local test database.

Tests insert unique fixture IDs, including ten competing inserts for one ID,
and delete only their fixtures. One test changes its own fixture balance to
verify that duplicate creation cannot overwrite it. No shared tables are
truncated. A killed test process may leave fixture rows behind.

### Apply the name and idempotency extension

With the test database already created, apply pending migrations and rerun:

```sh
goose -dir migrations postgres "$TEST_DATABASE_URL" up
go test -tags=integration -count=1 -v ./internal/adapters/postgres
```

Migration 2 preserves existing IDs/balances, backfills `Unnamed account` if
needed, and creates the account-creation replay table. Ordinary integration
tests never run migrations automatically.

To additionally check migration of empty and populated tables, explicitly run:

```sh
go test -tags=integration,migrationcheck -count=1 -run TestNameMigration -v ./internal/adapters/postgres
```

This separate opt-in check executes the migration SQL only in uniquely named
schemas inside transactions in `ledger_test`. It always rolls them back,
including the schemas and test rows; it does not change public tables or goose
version state.

## 7. Run the API yourself

Apply migrations separately to the development database, then start the server:

```sh
export DATABASE_URL='postgres://ledger:ledger@localhost:5432/ledger?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
go run ./cmd/api
```

Adjust the port for your environment. The default HTTP address is
`127.0.0.1:8080`; override `HTTP_ADDR` explicitly if needed. The server verifies
database connectivity but never applies migrations. It writes JSON logs to
stderr and stops gracefully with Ctrl+C.

In another terminal:

```sh
KEY="$(openssl rand -hex 16)"
curl -i http://127.0.0.1:8080/accounts \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $KEY" \
  --data '{"name":"Savings"}'
```

Use your existing random-key tool; OpenSSL is only an example, not a new
dependency to install. Repeat the same curl command with the same key to replay
the original 201 response. Change the name with that key to get a 422 conflict.
Use a new random key to deliberately create another account, even with the
same name.

```sh
curl -i http://127.0.0.1:8080/accounts
```

Names are labels, not users or ownership claims. Keys are shared across
callers for this endpoint and retained without expiry. They provide retry
safety, not authentication or privacy. On timeout, retry with the same key:
the database operation may already have committed.

## 8. Verify and initialize the fixed-supply ledger

First update the isolated test database and run the existing regressions:

```sh
export TEST_DATABASE_URL='postgres://ledger:ledger@localhost:5432/ledger_test?sslmode=disable'
goose -dir migrations postgres "$TEST_DATABASE_URL" up
go test -tags=integration -count=1 -v ./internal/adapters/postgres
```

Then explicitly select the initialization checks:

```sh
go test -tags=integration,initializationcheck -count=1 -v ./internal/adapters/postgres
```

This additional selector permits schema setup: its tests create uniquely named
schemas in `ledger_test`, apply migrations there, initialize only those schemas,
and drop only those schemas during cleanup. Public tables and goose migration
versions are not reset, and development data is never used. A killed process
may leave a test schema. Tests verify the database name before setup.

The checks cover competing initializers, no-op repeats, redistribution,
rollback after partial writes, invalid state, append-only/balanced history,
role backfill, and old stored response compatibility.

After review and successful tests, initialize your development ledger yourself:

```sh
export DATABASE_URL='postgres://ledger:ledger@localhost:5432/ledger?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
go run ./cmd/initledger
```

Adjust ports as needed. Migrations do not issue money. The last command is the
explicit one-time operation: it creates `system-origin` (`Supply origin`) at
-100,000,000 sititos and `system-reservoir` (`Reservoir`) at +100,000,000,
with one balanced opening transaction.

Repeat the same command to get `already initialized` without extra funds.
It refuses inconsistent state instead of repairing it. Existing ordinary
zero-balance accounts remain unchanged. The command has a 30-second deadline;
after a timeout or ambiguous connection failure, rerun against the same
database to determine whether initialization committed.

Restart the API after updating its code, then GET `/accounts` to see roles
and both system accounts. New ordinary accounts still start at zero. There
origin is not a spendable account. Transfers require explicit initialization.

Migration 3 is forward-only and its rollback deliberately fails to protect
history. Do not edit applied migrations or reset development data to work
around failures; report the error and investigate it.

## 9. Verify transfers

Apply migration 4 to the test database yourself, then run the ordinary
regressions and explicitly selected isolated initialization/transfer checks:

```sh
goose -dir migrations postgres "$TEST_DATABASE_URL" up
go test -tags=integration -count=1 -v ./internal/adapters/postgres
go test -tags=integration,initializationcheck -count=1 -v ./internal/adapters/postgres
```

The second test command permits schema setup as described above. Transfer
checks cover balanced history, conserved supply, successful response replay,
key scoping, insufficient-funds retries, concurrent overspending, opposite
directions, partial-write rollback, key-owner rollback, lock-wait timeouts,
HTTP behavior, and the distribution-only reservoir entry guard.

After those pass, apply migration 4 to development separately. If needed, run
the explicit initializer, then restart the API with the updated code. Choose
an existing ordinary account ID from GET /accounts:

```sh
goose -dir migrations postgres "$DATABASE_URL" up
go run ./cmd/initledger
go run ./cmd/api
```

In another terminal, generate a fresh random key using your existing tool,
then send:

```sh
curl -i http://127.0.0.1:8080/transfers \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $KEY" \
  --data '{"source_account_id":"system-reservoir","destination_account_id":"REPLACE_WITH_ACCOUNT_ID","amount_sititos":250}'
```

The transfer returns 201; repeat with the same key and payload to replay it
without spending again. GET /accounts shows updated balances. Transfers into
the reservoir or involving the origin return 422 account_not_transferable.
The reservoir only distributes; ordinary accounts can send to other ordinary
accounts. This project has no authorization, so it remains a local learning
exercise rather than a real-money API.
