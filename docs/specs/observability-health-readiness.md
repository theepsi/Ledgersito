# Observability: Health and Readiness

## Status

Approved and implemented.

## Purpose and scope

Learn the operational distinction between a process that is alive and one
currently able to serve API requests. Add simple, read-only HTTP health and
readiness checks as the first P3 observability slice.

- `GET /healthz` reports whether the HTTP process can answer a request. It must
  not depend on PostgreSQL or ledger initialization.
- `GET /readyz` reports whether the service is ready to handle API requests.
  It verifies that PostgreSQL is reachable and that Goose migration 5 is
  currently applied, using a bounded context. It does not change ledger state.
- These endpoints complement, but do not replace, ledger reconciliation.
- Add both endpoints to the hand-maintained OpenAPI contract.
- Add local probe examples and their database/initialization distinction to the
  public getting-started guide.

This specification covers only health and readiness. Structured request logs,
latency/error metrics, tracing, and external collection or dashboard tools
remain separate future specifications.

## HTTP behavior

- Both endpoints accept only `GET`. Other methods return `405` with
  `Allow: GET`; unknown paths retain the existing JSON `404` behavior.
- Reject any query string, including a bare `?`, with `400 invalid_request`.
- `GET /healthz` returns `200 OK` with `{"status":"ok"}` when the process can
  answer the request. It performs no database operation.
- `GET /readyz` checks database availability and the current application of
  Goose migration 5 with a request-scoped deadline no longer than five seconds.
  A missing migration record, a latest `is_applied = false` record for
  migration 5, a missing migration table, or an inaccessible database means
  not ready. On success, return `200 OK` with
  `{"status":"ready"}`.
- If readiness cannot be established because the database is unavailable or
  the check times out, return `503 Service Unavailable` with the existing JSON
  error envelope and code `not_ready`. Do not expose connection details,
  database errors, credentials, or query text.
- Responses must not disclose ledger balances, account data, initialization
  state, or internal topology. They must not mutate the database.
- Do not add authentication, external probes, background polling, or a new
  dependency for these checks.

## Acceptance scenarios

- The health endpoint returns success without opening a database connection.
- The readiness endpoint returns success when its configured database check
  finds migration 5 currently applied, even if the ledger has not been
  initialized.
- A database connection failure or readiness deadline returns `503 not_ready`,
  not a success-shaped response.
- A missing Goose migration table, a missing migration 5 record, or a latest
  migration 5 record marked unapplied returns `503 not_ready`.
- A blocked readiness check is bounded by its deadline and does not block
  unrelated HTTP requests beyond normal server resource limits.
- Neither endpoint changes accounts, entries, transactions, initialization
  metadata, or replay records.
- Unsupported methods return `405` and the correct `Allow` header; unknown
  paths continue to return `404`.
- Responses contain no database error details or ledger information.

Use focused HTTP tests with a controllable readiness dependency for success,
failure, deadline, method, and routing cases. Opt-in PostgreSQL coverage
verifies the migration-state check in an isolated schema. Database preparation
and migration execution remain user-controlled.

## Non-goals

- Determining whether the ledger has been initialized or reconciling its
  persisted invariants.
- Changing startup checks, graceful shutdown, or server configuration.
- Metrics, structured request logging changes, tracing, dashboards, alerting,
  deployment probes, or orchestration configuration.
- Account-level, transaction-level, or other sensitive diagnostic output.

## Implementation notes

- Readiness checks the most recent Goose history row for migration 5, the
  migration that added transaction posting timestamps. This confirms the
  schema version needed by the current API without requiring ledger
  initialization.
- The database error is logged server-side for diagnosis; the HTTP response
  contains only the stable `not_ready` code and a generic message.
