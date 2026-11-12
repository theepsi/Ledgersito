# Observability: Local Request Tracing

## Status

Approved and implemented.

## Purpose and scope

Learn how a trace connects timed spans belonging to one request. Logs describe
individual events; metrics summarize many requests; a trace shows the
relationship between one HTTP request and the storage operation it triggers.
Reconciliation remains independent of all three.

Provide a small, **single-process** tracing slice for the API:

- One root span for each handled HTTP request and, when applicable, one child
  span for its outer PostgreSQL adapter operation.
- Opt-in tracing, disabled by default, with completed spans written as
  structured JSON records to the API's existing stderr logger. This permits
  inspecting traces locally without a collector or dashboard.
- No trace headers, cross-service propagation, external exporter, persistent
  trace storage, or installation in this slice. These are potential separate
  future specifications.

No new dependency or collector is required.

## Trace behavior

- Enable tracing only when `TRACE_ENABLED=true`; unset, empty, or `false`
  disables it.
  Reject any other value at startup with a clear error that does not expose
  configuration values. When disabled, emit no span records and preserve
  existing logs, metrics, and HTTP behavior.
- When enabled, trace every normally completed request, including health,
  readiness, metrics, validation errors, unknown paths, and unsupported
  methods. Do not recover panics or change the server's panic handling.
- Generate a new opaque trace ID and root span ID for each request. A storage
  span has the same trace ID, a distinct span ID, and the root's span ID as
  its parent. A request that never reaches storage has no storage span. Do not
  accept or send caller-provided trace IDs or tracing headers.
- Name root spans using the existing canonical request-log route labels or
  `unmatched`. Name storage spans using the fixed operation labels from the
  [metrics spec](observability-metrics.md). In particular, `/metrics` remains
  `unmatched` in the existing request-log route scheme.
- A storage span covers the outer adapter call from entry through cleanup,
  including connection setup, transaction work, and callbacks. It is not a
  per-SQL span or a measurement of database-server execution time.
- Record one completed span event per span, with `event: span_completed`,
  `trace_id`, `span_id`, `parent_span_id` for child spans, `span_kind`,
  `span_name`, UTC `start_time`, `duration_ms`, and `outcome`. The trace ID is
  32 hexadecimal characters and each span ID is 16. IDs are opaque random
  values and do not encode ledger or caller data.
- For the root span, classify HTTP 5xx as `error` and other completed
  responses as `ok`. For storage spans, classify a returned error as `error`
  and no error as `ok`. A storage error can be a business rejection or
  callback failure, not necessarily a PostgreSQL failure.
- If an ID cannot be generated, do not produce a partial trace: report the
  tracing failure without sensitive details and continue serving the request
  with unchanged API behavior. Do not claim an unrecorded trace succeeded.
- Tracing adds work and stderr output; it must not change response status,
  payload, transaction semantics, deadlines, idempotency, or existing metrics.
  Synchronous logging can add latency or block if stderr is slow.

The output is structured local span records, **not** an
OpenTelemetry or distributed-tracing export format. Existing request logs do
not gain trace IDs in this slice; the trace ID groups the span records.

## Privacy and resource limits

- Never put raw URL paths, query strings, headers, bodies, account names or
  IDs, transaction IDs, idempotency keys, SQL, database URLs, client
  addresses, or raw error text in span records.
- Span names and outcome values use fixed allowlists; unknown routes use
  `unmatched`.
- Keep only the in-flight context for a request; do not retain an unbounded
  in-process span history or add an HTTP trace-list endpoint.
- Span output follows the existing logger's destination and retention behavior:
  stderr is not durable storage unless the user separately captures it.

## Illustrative trace

For `GET /readyz`, the root span `http /readyz` contains the child
`storage readiness`. Both share a trace ID; the child references the root's
span ID. If `/healthz` is requested, there is only an HTTP root span. A trace
does not prove that the ledger's balances or invariants are correct.

## Acceptance scenarios

- Unset, empty, or `false` tracing configuration emits no span events; `true`
  emits them; invalid configuration fails startup without exposing its value.
- A normally completed readiness request yields one HTTP root and one storage
  child with matching trace IDs, distinct span IDs, and the correct parent.
  Health and rejected requests that do not access storage yield only a root.
- Concurrent requests have independent trace IDs and do not mix parent-child
  relationships. Normal spans end exactly once, including storage errors and
  HTTP validation failures.
- Root and storage outcomes follow the classification above; a business
  rejection is not described as proof of a database failure.
- Durations are nonnegative and measured using a monotonic clock. Span names,
  attributes, and emitted JSON contain no sensitive caller or ledger values.
- A tracing ID-generation failure is reported safely and does not change the
  HTTP response or ledger operation. Existing logs and metrics still work.
- Tracing adds no network calls, database mutations, response headers, or
  extra public endpoint, and preserves cancellation and timeout behavior.

Use focused HTTP and adapter tests with controllable ID generation and a
captured sink. Cover disabled/enabled configuration, nesting, failures,
privacy, concurrency, and existing response behavior; use opt-in PostgreSQL
integration tests only under the existing database-preparation agreement.

## Implementation notes

- The API emits a child span first and its parent second after the request
  completes. It buffers at most one storage span per request and does not
  retain completed traces.
- If an unexpected second storage span is attempted for a single request, the
  whole trace is dropped and a generic tracing error is logged rather than
  emitting a partial trace.
- Use the [getting-started guide](../getting-started.md#try-local-request-tracing)
  to enable tracing, make read-only requests, and inspect the resulting records.

## Non-goals

- A trace collector, UI, dashboard, alerting, persistent storage, or deployment
  setup.
- Distributed context propagation, SQL-level spans, or database query text.
- Tracing initialization/reconciliation commands or changing ledger behavior.
