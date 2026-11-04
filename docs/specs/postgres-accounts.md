# PostgreSQL Account Storage

The original create-only adapter below is implemented and verified.
[Account Names](account-names.md) extends its schema and insert query; the
API additionally uses transactional [idempotency](account-idempotency.md).

## Purpose and scope

Implement the storage boundary of [Create Account](create-account.md) using
PostgreSQL. Learn database-enforced uniqueness, SQL-to-Go code generation,
migrations, and real-database integration testing.

The adapter implements the application's existing `Store` interface. It does
not generate IDs, serve HTTP requests, or change account domain rules.

## Schema and tooling

- Introduce the `accounts` table through a versioned goose migration.
- `id` is `TEXT PRIMARY KEY`, preserving accepted domain IDs exactly.
  Equality is case-sensitive: `account-A` and `account-a` can coexist.
- `balance` is `BIGINT NOT NULL DEFAULT 0`, measured in sititos.
- SITO is implicit for this single-currency ledger; no currency column is
  needed in this scope.
- Names, timestamps, and negative-balance restrictions are deferred.
- Use sqlc-generated pgx query code inside the PostgreSQL adapter. The domain
  and application layers must not import generated queries or pgx types.
- Migrations are an explicit setup step, not something `Create` runs.

## Creation behavior

- A valid, newly created domain account is inserted with its exact ID and
  zero sititos.
- Return nil only after PostgreSQL reports a successful insert. The row must
  then be observable through an independent database query.
- Creation is insert-only. Do not update an existing row, use an upsert that
  overwrites it, or silently ignore a duplicate.
- Enforce uniqueness atomically through the primary key, not a preliminary
  existence query.
- Concurrent attempts to create the same ID yield exactly one success.
  All other attempts report an identifiable ID conflict.
- Forward the supplied context to the database operation. Cancellation and
  database failures must not be reported as success.

## Errors and uncertain outcomes

- Map a violation of the account ID primary key to an error identifiable as
  `createaccount.ErrAlreadyExists` through `errors.Is`.
- Do not classify all database errors, or all possible constraint violations,
  as duplicate account IDs.
- Other failures retain their underlying identity through wrapping, so callers
  can inspect them using `errors.Is` or `errors.As` as appropriate.
- Do not retry, panic on ordinary database failures, or claim that every error
  proves no row was persisted. A connection failure can leave the outcome
  uncertain, as described in the application spec.
- Failed duplicate creation leaves the existing row unchanged.

## Acceptance examples

| Scenario | Expected outcome |
|---|---|
| Create `account-1` | Nil error; independent query finds exactly that ID and zero sititos |
| Create `account-A` and `account-a` | Two distinct rows, each with zero sititos |
| Create an ID containing non-ASCII, non-whitespace characters | ID preserved exactly |
| Create an existing ID | Identifiable conflict; existing row unchanged |
| Concurrently create one ID | Exactly one success, all other results identifiable conflicts, one stored row |
| Database access fails | Non-nil, inspectable error; not falsely reported as a duplicate |
| Operation receives an already-canceled context | Identifiable cancellation error; not a successful creation |

Use a bounded concurrent test, such as ten attempts, rather than a load test.
Read persisted state directly in integration tests; an application read use
case is not required for this spec.

## Test execution and isolation

- Integration tests use a separate test database in the existing local
  PostgreSQL container, not the development database.
- Provide exact database preparation and migration commands for the user to
  execute. Do not automatically start Docker, create a database, or apply
  migrations without approval.
- Make database tests opt-in with an explicit integration-test selector and a
  dedicated `TEST_DATABASE_URL`. Ordinary unit tests do not connect to a DB.
- When integration tests are selected, missing configuration or an unreachable
  database fails clearly rather than skipping or passing.
- Validate the database target before test writes and refuse the development
  database. Document the expected test database name.
- Give each test run distinct fixture IDs and remove only its own rows. Do not
  truncate shared tables, drop databases, or clean up development data.
- Verify the create-only storage contract against real PostgreSQL, not only
  mocks. Existing application/domain tests must continue to pass without it.

## Non-goals

- Account names and metadata.
- Concrete ID generation and HTTP endpoints.
- Reading accounts as an application feature.
- Updating balances, funding, transfers, or ledger entries.
- Idempotency, automatic retries, and uncertain-outcome recovery.
- Overdraft policy or enforcing every domain validation rule in SQL.
- Automatic container provisioning or new testcontainers dependencies.
