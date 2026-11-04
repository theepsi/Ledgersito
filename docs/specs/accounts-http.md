# Accounts HTTP API

## Status and scope

Agreed response and input-validation rules for the account API. Not implemented.
Key validation, resource limits, timeouts, and migration of existing accounts
still need agreement before implementation.

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
  "balance": {
    "sititos": 0,
    "currency": "SITO"
  }
}
```

A successful same-key replay returns the original status 201 and body.
Do not reconstruct the response from a later account balance.

## GET /accounts

Return status 200 and the response defined in the listing spec. Items use the
same account shape as the POST response. No idempotency key is required.

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
| Committed key reused with a different normalized payload | 422 | `idempotency_key_reused` |
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

## Remaining decisions

- Accepted media types and response Content-Type.
- Key format, length, and repeated header handling.
- Request body size and operation timeouts, including wait timeout responses.
- Unsupported methods, paths, and query parameters.
- Concrete ID generation and minimal server configuration.
- Account-name migration and backfill policy for existing accounts.
