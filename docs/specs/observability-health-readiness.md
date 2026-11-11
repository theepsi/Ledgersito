# Observability: Health and Readiness

## Status

Draft — not approved for implementation.

## Purpose and scope

Learn the operational distinction between a process that is alive and one
currently able to serve API requests. Add simple, read-only HTTP health and
readiness checks as the first P3 observability slice.

- `GET /healthz` reports whether the HTTP process can answer a request. It must
  not depend on PostgreSQL or ledger initialization.
- `GET /readyz` reports whether the service is ready to handle API requests.
  It checks the database using a bounded context and does not change ledger
  state.
- These endpoints complement, but do not replace, ledger reconciliation.

This specification covers only health and readiness. Structured request logs,
latency/error metrics, tracing, and external collection or dashboard tools
remain separate future specifications.

## HTTP behavior

- Both endpoints accept only `GET`. Other methods return `405` with
  `Allow: GET`; unknown paths retain the existing JSON `404` behavior.
- `GET /healthz` returns `200 OK` with `{"status":"ok"}` when the process can
  answer the request. It performs no database operation.
- `GET /readyz` checks database availability with a request-scoped deadline no
  longer than five seconds. On success, return `200 OK` with
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
  succeeds.
- A database connection failure or readiness deadline returns `503 not_ready`,
  not a success-shaped response.
- A blocked readiness check is bounded by its deadline and does not block
  unrelated HTTP requests beyond normal server resource limits.
- Neither endpoint changes accounts, entries, transactions, initialization
  metadata, or replay records.
- Unsupported methods return `405` and the correct `Allow` header; unknown
  paths continue to return `404`.
- Responses contain no database error details or ledger information.

Use focused HTTP tests with a controllable readiness dependency for success,
failure, and deadline cases. Add opt-in PostgreSQL coverage only if needed to
verify the production database check. Database preparation and migration
execution remain user-controlled.

## Non-goals

- Determining whether the ledger has been initialized or reconciling its
  persisted invariants.
- Changing startup checks, graceful shutdown, or server configuration.
- Metrics, structured request logging changes, tracing, dashboards, alerting,
  deployment probes, or orchestration configuration.
- Account-level, transaction-level, or other sensitive diagnostic output.

## Open decision

The readiness signal should reflect the API's actual ability to serve requests.
A database ping confirms connectivity but not that the expected schema is
available. Before implementation, agree whether readiness should check only
connectivity or also verify the required migration/schema state. In either
case, readiness must not require ledger initialization.
