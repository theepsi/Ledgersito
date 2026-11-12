# Observability: Structured HTTP Request Logs

## Status

Draft — not approved for implementation.

## Purpose and scope

Learn how structured request logs make HTTP service behavior inspectable
without collecting ledger or caller data. Add one structured completion event
for each request handled by the API server.

- Use the existing Go `log/slog` logger and JSON output; do not add a logging
  dependency or change log destinations.
- Keep this slice to HTTP request completion events. Existing startup,
  shutdown, application-error, and database diagnostic logs are not replaced
  by this specification.
- Health/readiness behavior remains as defined in the
  [health/readiness spec](observability-health-readiness.md).
- Latency/error metrics and distributed tracing remain separate future
  specifications.

## Event fields and privacy

Each completion event has a stable event name and includes:

- HTTP method.
- A low-cardinality route label: the matched API path (`/accounts`,
  `/transfers`, `/transactions`, `/analytics`, `/healthz`, or `/readyz`), or
  `unmatched` for unknown paths.
- Final HTTP status code.
- Request duration in milliseconds, measured with a monotonic clock.

Do not log raw URLs or query strings, request/response bodies, headers,
account IDs, names, transaction IDs, idempotency keys, database URLs, or client
IP addresses. In particular, do not use caller-controlled values as event
fields or route labels. Do not copy application or database error text into
the request completion event.

Keep the existing JSON logger and structured application-error logs. The
completion event supplements them; it does not change API responses, retry
semantics, or error classification.

## Severity

- Use `INFO` for responses below 400.
- Use `WARN` for 4xx responses.
- Use `ERROR` for 5xx responses.

Do not change how Go's `net/http` server handles panics in this slice. Request
events describe responses the handler completed; behavior when a handler
panics before completing a response remains outside this specification.

## Acceptance scenarios

- Each normal request handled by the API produces exactly one completion event,
  including successful, validation-error, not-found, method-not-allowed, and
  readiness-failure responses.
- The event contains the method, canonical route label (or `unmatched`),
  final status, duration, and stable event name.
- Unknown paths and caller-supplied path/query values never appear in the
  route label or as raw URL data.
- Events are valid structured JSON on the existing stderr log stream.
- Status-based severity follows the rules above.
- No request body, response body, headers, account data, idempotency key,
  database URL, query string, or client IP is present in the event.
- Existing API responses and existing application-error logging behavior are
  unchanged.

Use focused HTTP tests with a captured `slog` output to verify event count,
fields, severity, duration presence, route normalization, and absence of
request-provided sensitive values. Do not add log assertions for unrelated
existing application-error messages.

## Non-goals

- Request IDs, client-provided correlation headers, or response headers added
  for correlation.
- Metrics, traces, dashboards, alerting, log shipping, or deployment changes.
- Changing database logging, command-line tool logging, or existing
  application-error messages.
- Logging request/response payloads, caller identity, network addresses, or
  ledger identifiers.
