# Observability: HTTP and Storage Metrics

## Status

Draft — not approved for implementation.

## Purpose and scope

Learn how counters and latency distributions describe service behavior over
time, and how they differ from individual request logs and ledger analytics.
Propose a small, process-local set of read-only HTTP and PostgreSQL adapter
metrics. Reconciliation remains the tool for checking ledger invariants.

This draft proposes a pull-based `/metrics` endpoint in Prometheus text format,
without requiring a collector, dashboard, or running external service. The
exposition format and any library/dependency need review before implementation;
no installation is authorized by this draft.

## Proposed measurements

Measure durations in seconds using a monotonic clock. Counters and histograms
are cumulative since process start and reset on restart; they are not durable
ledger data. Concurrent updates and scrapes must be safe.

**HTTP requests:** count completed requests and record their durations,
including validation failures, unknown paths, and health/readiness requests.
Use the existing fixed route names (`/accounts`, `/transfers`, `/transactions`,
`/analytics`, `/healthz`, `/readyz`) or `unmatched`. Group status as `2xx`,
`3xx`, `4xx`, or `5xx`; use `other` for other final statuses. Group methods as
`GET`, `POST`, `HEAD`, or `other` so caller-provided method strings cannot
create unbounded labels. Requests that panic before normal completion are
outside this first slice, matching the current request-log behavior.

**PostgreSQL adapter operations:** count completed API storage attempts and
record their durations for the fixed operations `create_account`,
`list_accounts`, `transfer`, `list_transactions`, `analytics`, and
`readiness`. Time each outer adapter call from entry through its cleanup,
including connection setup, transaction work, callbacks, and cleanup. This is
*adapter-operation latency*, not the latency of an individual SQL query or
pure database-server execution time.

Label each storage attempt `success` when it returns no error and `error` when
it returns an error. This measures **operation errors**, not exclusively
database failures: business rejections, timeout/cancellation, callback errors,
and connection/SQL/cleanup failures can all return errors. Do not claim that
the error count alone identifies a database outage. A retry or successful
idempotent replay is another attempt, not another ledger transaction.
Initialization and reconciliation CLI operations are outside this API slice.

## Proposed HTTP exposition

- `GET /metrics` returns a valid Prometheus text exposition with HTTP and
  adapter-operation counters and duration histograms. It does not access
  PostgreSQL, initialize the ledger, or change ledger data.
- Other methods return the existing JSON `405 method_not_allowed` with
  `Allow: GET`; reject query strings, including a bare `?`, with the existing
  JSON `400 invalid_request`. Paths match exactly, as with existing routes.
- Exclude all requests to the exact `/metrics` path from HTTP request metrics,
  including invalid methods and queries, so scraping does not change the
  measurements being scraped. Existing request logging remains unchanged.
- Exposition must contain only fixed, bounded labels. Never include account IDs,
  names, transaction IDs, idempotency keys, raw paths, query strings, client
  addresses, SQL, database URLs, or error messages.
- This endpoint is unauthenticated like the rest of the learning API; keep the
  existing loopback default and do not imply production exposure is safe.

Illustrative observation after one successful health check, using provisional
names (histogram bucket, sum, and count lines omitted here):

```text
ledgersito_http_requests_total{method="GET",route="/healthz",status_class="2xx"} 1
```

## Acceptance scenarios

- A completed request increments exactly one HTTP counter and one histogram
  observation for its normalized method, route, and status class.
- A readiness check increments both HTTP and readiness adapter observations;
  a health check does not create an adapter observation.
- Successful, rejected, and failed storage calls are counted once each;
  returned business errors are not mislabeled as proven database failures.
- A replay counts as an attempt; it does not imply a new ledger transaction.
- Unknown methods and paths normalize to `other` and `unmatched`; sensitive
  request or database data never appears in metric names or labels.
- Scrapes and rejected requests to the exact `/metrics` path do not change HTTP
  metric counts. Scraping does not query or mutate the ledger.
- Concurrent recording and scraping does not race, corrupt exposition, or
  alter API response and idempotency behavior.
- Restarting the process resets the in-memory metrics.
- The new endpoint is reflected in the tracked OpenAPI contract and public
  documentation when implemented.

Use focused HTTP and adapter tests with controllable dependencies and a
deterministic clock where useful. Verify successful and failed operations,
bounded labels, concurrent updates, histogram observations, and exposition.
Any optional PostgreSQL integration test must follow the existing
user-controlled database preparation agreement.

## Decisions for review

- Confirm Prometheus text exposition versus another way to inspect metrics,
  and whether a metrics library is appropriate. Any dependency installation
  requires separate user permission.
- Agree final metric names and fixed histogram bucket boundaries before
  implementation; buckets must include a `+Inf` bucket.
- Confirm that storage *operation* errors are sufficiently useful as a first
  learning step, or separately specify instrumentation that distinguishes
  actual database failures from business and callback errors.

## Non-goals

- Installing or configuring Prometheus, dashboards, collectors, alerts, or
  deployment infrastructure.
- Distributed tracing, request IDs, per-account metrics, raw query
  instrumentation, or modifying existing logs.
- Persistent counters, historical ledger analytics, or reconciliation.
- Migrations, database initialization, and changes to ledger behavior.
