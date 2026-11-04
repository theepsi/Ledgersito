# Accounts HTTP API

## Status and scope

Agreed response and input-validation rules for the account API. Implemented;
real-database verification of this extension is pending.
Key validation, resource limits, and operation timeouts are agreed below.
The name migration policy is defined in
[Account Names](account-names.md).

This contract extends [Account Creation Idempotency](account-idempotency.md)
and [List Accounts](list-accounts.md).

## POST /accounts

Requires an `Idempotency-Key` and a JSON object containing only `name`:

```json
{"name":"Dragon's savings"}
```

- Require exactly one name field with a string value.
- Reject malformed JSON, unknown fields, duplicate fields, trailing JSON,
  non-object bodies, missing names, and invalid names.
- Name validation and trimming follow [Account Names](account-names.md).
- Reject invalid input without creating an account or reserving the key.
- Successful creation returns status `201 Created` with the account object:

```json
{
  "id": "generated-id",
  "name": "Dragon's savings",
  "role": "ordinary",
  "balance": {
    "sititos": 0,
    "currency": "SITO"
  }
}
```

A successful same-key replay returns the original status 201 and body.
Do not reconstruct the response from a later account balance.
Successful responses stored before roles were introduced replay unchanged and
may lack `role`. Newly recorded creations include `ordinary`; callers cannot
choose a role.

## GET /accounts

Return status 200 and the response defined in the listing spec. Items use the
same account shape as the POST response. No idempotency key is required.
Listings always include `role` and include the origin and reservoir after
explicit [initialization](ledger-initialization.md).

## Keys, media types, and limits

- POST requires exactly one `Idempotency-Key` header value containing 1-128
  ASCII letters, digits, hyphens, or underscores. Keys are case-sensitive.
  Do not trim or otherwise normalize the value received by the handler.
- Missing, empty, invalid, or repeated keys are rejected before account
  creation or key reservation. A comma-separated value is not a valid key.
- Clients should generate random keys rather than predictable counters.
- POST requires `Content-Type: application/json`, optionally with
  `charset=utf-8` (case-insensitive). Reject missing or unsupported media types
  and unsupported parameters.
- Limit the POST body to 4096 bytes, including JSON whitespace. A body of
  exactly 4096 bytes is allowed if otherwise valid.
- Responses defined by this contract use `Content-Type: application/json`.
- GET and POST application/database operations have a five-second timeout,
  including waiting for an active same-key transaction. Propagate cancellation
  to database work. This is an operation deadline, not a complete policy for
  slow HTTP connections.
- If that deadline expires before a response is committed and the client is
  still connected, return status 503 with code `request_timeout`.
- Timeout does not prove that the database rolled back. Retry POST with the
  same key to recover its outcome; never suggest using a new key.
- These limits are not authentication, authorization, or rate limiting.

## Errors

Use an error envelope containing a stable machine-readable code and a
human-readable message:

```json
{
  "error": {
    "code": "invalid_name",
    "message": "Name must contain 1-100 Unicode code points after trimming."
  }
}
```

| Condition | HTTP status | Error code |
|---|---|---|
| Invalid JSON or request shape | 400 | `invalid_request` |
| Invalid name | 400 | `invalid_name` |
| Missing idempotency key on POST | 400 | `missing_idempotency_key` |
| Empty, invalid, or repeated idempotency key | 400 | `invalid_idempotency_key` |
| POST body exceeds 4096 bytes | 413 | `request_too_large` |
| Missing or unsupported POST Content-Type | 415 | `unsupported_media_type` |
| Committed key reused with a different normalized payload | 422 | `idempotency_key_reused` |
| Application/database operation deadline exceeded | 503 | `request_timeout` |
| Unexpected internal failure | 500 | `internal_error` |

Messages explain the failure without exposing SQL, credentials, database
errors, or other internal details. Clients should branch on codes, not message
text. Names are not unique; sharing a name is not a conflict.

## Acceptance scenarios

- Valid POST returns status 201 and the trimmed, case-preserved name, generated ID,
  zero sititos, and SITO currency.
- Same-key equivalent requests replay the original status and body.
- Different-key requests with the same name can create distinct accounts.
- Different-payload reuse of a committed key returns 422.
- Invalid bodies return 400 and do not reserve a key.
- Duplicate JSON name fields are rejected, even if their values match.
- GET uses the agreed accounts envelope; POST returns one account directly.
- Unexpected failures return a generic 500 error, never a successful result.
- Key lengths 1 and 128 are accepted; empty and 129-character keys, invalid
  characters, and repeated header values are rejected.
- Bodies above 4096 bytes return 413 without reserving a key.
- JSON with an optional UTF-8 charset is accepted; unsupported media types or
  parameters return 415.
- Deadline expiration while waiting for a competing operation returns 503,
  not a replay or a claim that the competing operation rolled back.

## Routing and server

- Match exactly `/accounts`, without trailing-slash or encoded-path aliases.
- Unknown paths return JSON 404 `not_found`. Unsupported methods, including
  HEAD, return 405 `method_not_allowed` with `Allow: GET, POST`.
- Reject query parameters (including a bare `?`) with 400 `invalid_request`.
- Generate opaque account IDs with standard-library `crypto/rand.Text()`.
  The database primary key remains the final uniqueness safeguard.
- `cmd/api` requires `DATABASE_URL`. Default `HTTP_ADDR` is `127.0.0.1:8080`;
  callers can explicitly override it. No implicit `.env` loading.
- Verify database connectivity at startup; report failures and exit nonzero.
- Log structured JSON, shut down gracefully on SIGINT/SIGTERM, and never
  apply migrations or start PostgreSQL automatically.
- Bound HTTP header reading to five seconds, request reading to ten seconds,
  response writing to fifteen seconds, and idle connections to sixty seconds.
  Graceful shutdown allows ten seconds for active requests.
