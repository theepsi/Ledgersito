# API and Public Documentation

## Status and purpose

Approved and implemented. The user approved the documentation scope, local
and CI validation, and downloading the pinned development-only validator.
The approved local HTML extension renders that contract for browser reading;
it does not add a hosted viewer or request console.

Make the existing educational ledger understandable and usable from a fresh
checkout using tracked documentation alone. Learn how to describe an HTTP
contract accurately, distinguish executable setup instructions from design
explanations, and keep public documentation aligned with tested behavior.

This feature documents the existing system. It does not change API behavior,
ledger rules, database schemas, or deployment expectations.

## Deliverables

| File | Purpose |
|---|---|
| `docs/openapi.yaml` | Hand-maintained OpenAPI 3.1.0 contract for the existing API. |
| `docs/getting-started.md` | Public local setup, example requests, and verification instructions. |
| `docs/architecture.md` | Public explanation of code boundaries and ledger correctness mechanisms. |
| `README.md` | Concise entry point linking to the tracked contract and public guides. |
| `redocly.yaml`, `Makefile`, `.github/workflows/ci.yml` | Matching local and CI validation of the OpenAPI contract and examples. |
| `make docs-openapi` | Local browser-readable reference generated at ignored `docs/openapi.html` from the YAML contract. |

Keep detailed behavioral specifications in `docs/specs/` as the source of
truth. The public guides explain those rules without introducing competing
contracts. Check the implemented handlers and tests as well as approved specs;
report discrepancies rather than silently changing the API or guessing which
behavior was intended.

Do not read, edit, copy, stage, rename, delete, or add ignore rules for the
local untracked `docs/DESIGN.md`, `docs/SETUP.md`, or
`go-ledger-project-startup.md`. Write the public guides from tracked sources.
Replace README links to the local guides only when the public replacements
are implemented.

## OpenAPI contract

Cover seven operations across the six API paths:

| Operation | Success | Main behavior |
|---|---|---|
| `POST /accounts` | `201` | Create an ordinary account or replay the original successful response. |
| `GET /accounts` | `200` | List accounts, including system accounts, ordered by name then ID. |
| `POST /transfers` | `201` | Transfer existing funds atomically or replay the original successful response. |
| `GET /transactions` | `200` | List posted transactions and signed entries in deterministic entry-ID order. |
| `GET /analytics` | `200` | Report lifetime or bounded transfer flows and current stock metrics. |
| `GET /healthz` | `200` | Report process health without depending on PostgreSQL. |
| `GET /readyz` | `200` | Report readiness only when PostgreSQL is reachable and migration 5 is applied. |

For each operation, describe inputs, success/error schemas, applicable headers,
examples, and the statuses/codes from its existing feature spec. Use reusable
schemas for common amount and error objects without incorrectly equating all
numeric representations or all account responses.

### Request validation and routing

- Document exact paths, no trailing-slash or encoded-path aliases, JSON
  `404 not_found`, and unsupported-method `405 method_not_allowed` responses
  with each route's `Allow` header. HEAD is not an alias for GET.
- Only analytics accepts query parameters. Document the existing rejection
  rules, including a bare query delimiter and duplicate/unknown parameters.
  Health and readiness reject all query strings, including a bare `?`.
- Describe the required POST media type, permitted UTF-8 charset, 4096-byte
  body limit, strict request fields, and relevant name/account-ID validation.
- Explain restrictions OpenAPI schemas cannot fully enforce: duplicate JSON
  members, repeated headers, malformed Unicode, exact query multiplicity,
  integer token notation, and byte-based body limits.
- Do not imply that schema validation alone is equivalent to handler
  validation.

### Responses, amounts, retries, and time

- Represent existing numeric amounts and entry IDs with their actual JSON
  types. Document signed int64 sititos and positive int64 transfer requests;
  transfer amounts reject fraction, exponent, and quoted notation.
- Describe analytics cumulative volumes as nonnegative decimal strings, not
  int64 JSON numbers. Current balance/supply amounts remain numeric.
- Account listings require `role`. Newly created account responses include
  it, but previously stored account-creation responses may replay without
  `role`. Model that compatibility without weakening the listing schema.
- Describe exactly one case-sensitive `Idempotency-Key` value for POST,
  its permitted characters and length, separate account/transfer key scopes,
  original successful-response replay, payload conflicts, and non-reservation
  of failed attempts.
- Document five-second operation deadlines and the existing JSON error
  envelope. A POST timeout does not prove rollback: retry with the same key,
  not a new one. Do not promise that server-wide connection failures use the
  operation-level JSON envelope.
- History includes the opening transaction and does not expose `posted_at`.
  Entry-ID ordering is deterministic but is not commit-time ordering.
- Analytics requires initialization. Its `period` is either `"lifetime"` or
  an object with UTC-normalized `from` and `to` strings. Bounds must both be
  present, have explicit RFC 3339 offsets, and satisfy `from < to`.
- Describe the half-open window `from <= posted_at < to`. Lifetime flows
  include legacy untimestamped transfers; bounded flows exclude them.
  Timestamp coverage is whole-ledger coverage in either response, and current
  stock metrics are not historical balances at the window end.

Use the existing response examples and error matrices in
[Accounts HTTP API](accounts-http.md), [Transfers](transfers.md),
[Transaction History](transaction-history.md), and
[Ledger Analytics](ledger-analytics.md).

The local server example is `http://127.0.0.1:8080`. State that authentication
and authorization are absent; do not declare a fictitious security mechanism
or imply that this is a production service. No endpoint serves the OpenAPI
file or an interactive documentation UI.

### Local HTML reference

`docs/openapi.yaml` remains the hand-maintained machine-readable contract.
`make docs-openapi` validates it first, then uses the same pinned Redocly CLI
to render `docs/openapi.html`. Edit YAML, not generated HTML, and rerun the
command after contract changes.

Bundle the viewer scripts into the output and disable Google Fonts so the
page can be opened locally without a CDN or documentation server. Keep only
the YAML and build instructions in version control; ignore the exact generated
HTML path without changing rules for local private guides.

The page provides browsable endpoints, schemas, and examples, not a "Try it
out" console. No login, hosted upload, API call, new package installation, Go
runtime change, or CI publication is required. The initial npm tool download
still requires network access if it is not already cached.

## Public setup guide

### Prerequisites and environment

- Read the Go requirement from `go.mod`; use PostgreSQL 17 consistently with
  Compose and CI.
- Describe Docker Compose as the provided local database option and Goose
  and curl as tools used by the walkthrough. Separate development checks
  requiring sqlc or golangci-lint from running the API.
- Link to authoritative installation instructions or explain requirements;
  do not introduce automated installers or require AUR packages.
- Explain `.env.example`, copying it only when `.env` does not already exist,
  and explicit shell environment loading. The Go commands do not implicitly
  load `.env`.
- Use only documented local example credentials. Do not copy real `.env`
  contents, local account IDs, host-specific paths, or existing ledger data.
- Explain `POSTGRES_PORT`, `DATABASE_URL`, `TEST_DATABASE_URL`, and
  `HTTP_ADDR`, including the default loopback API address.

### Walkthrough and data safety

Describe the existing Compose service, its persistent volume, readiness,
explicit migrations, explicit fixed-supply initialization, and API startup.
Label every command that changes schema or data and the database it targets.
The API must not be described as starting PostgreSQL, migrating, or
initializing automatically.

Show how to create two ordinary accounts, capture their returned IDs, fund
one from the reservoir, and make an ordinary-to-ordinary transfer. Include
GET requests for accounts, history, lifetime analytics, and bounded analytics,
plus representative validation-error and idempotent-retry examples.

For a fresh initialized ledger, a 250-sitito distribution followed by a
70-sitito ordinary-to-ordinary transfer should produce:

| Metric | Expected value |
|---|---|
| Transfer count, excluding opening | `2` |
| Total transfer volume | `"320"` sititos |
| Reservoir distribution | `"250"` sititos |
| Ordinary-to-ordinary volume | `"70"` sititos |
| Current funds in circulation | `250` sititos |
| Current reservoir balance | `99999750` sititos |

Explain that these absolute totals assume fresh data; an existing ledger
retains its prior activity. Require distinct keys for new operations, and
reuse the exact key/payload when demonstrating retries. Do not reset a ledger
to make example totals match.

Describe reconciliation as a separate read-only CLI, including its exit codes
and the distinction between `go run` and a built executable. Explain stopping
the API and database without deleting the persistent volume. Do not present
volume deletion, database resets, or `goose down` as routine setup or cleanup;
ledger-history migrations are forward-only.

### Verification commands

- Include unit tests, formatting checks, vet, lint, and sqlc regeneration;
  distinguish tools required by each command.
- Explain that the provided Compose service creates `ledger`, not
  `ledger_test`. Preparing the disposable test database is an explicit,
  separate step.
- Integration commands must use `TEST_DATABASE_URL` pointing to `ledger_test`,
  never `DATABASE_URL`.
- Include the existing `integration`, `integration,initializationcheck`, and
  `integration,migrationcheck` selectors from CI. Explain which checks use
  isolated schemas and apply migrations there.
- Distinguish instructions from actions: writing the guide does not authorize
  installing tools, starting services, applying migrations, or running
  database-modifying examples.

## Public architecture guide

Explain domain, application, and adapter responsibilities using real package
and CLI paths. Include a small text or Mermaid diagram with no rendering
dependency.

Cover exact SITO/sitito arithmetic, accounts as ledger accounts rather than
users, origin/reservoir/ordinary roles, fixed-supply initialization, balanced
entries versus cached balances, atomic transfers and replay records, ordered
account locking, and immutable posted history.

Explain Goose migrations and sqlc-generated queries, forward-only history
preservation, legacy `NULL` timestamps, and `posted_at` as database wall-clock
posting observation rather than exact commit time/order.

Distinguish snapshot-consistent analytics from reconciliation: analytics
assumes existing ledger invariants, while reconciliation independently checks
them. Explain flow versus stock and why repeated circulation can make volume
exceed supply.

State the educational limitations, lack of authentication/authorization,
absence of a security-review claim, and non-production/non-real-money scope.
Do not add design proposals for pending roadmap features.

## Acceptance criteria and validation

- All public entry-point links resolve to tracked files from a fresh checkout;
  none require the private local guides.
- The OpenAPI document covers all five operations and matches actual response
  envelopes, field types, errors, request constraints, and retry behavior.
- Validate the contract semantically with the approved OpenAPI 3.1-capable
  Redocly CLI. YAML parsing alone is not OpenAPI validation. Run
  `make lint-openapi` locally and in its separate CI job.
- `make docs-openapi` validates and generates a locally openable HTML reference
  for all five operations. Scripts are inline, Google Fonts are disabled, and
  `docs/openapi.html` is ignored. Do not hand-maintain a second contract.
- Cross-check examples with existing HTTP/application tests. Treat any
  conflict as a review finding, not permission to change runtime behavior.
- Setup instructions match `compose.yaml`, `.env.example`, `go.mod`, the
  Makefile, CLI behavior, migrations, and current CI selectors.
- A later clean-checkout walkthrough, if authorized, uses disposable data,
  verifies the example flow and error/retry cases, and never resets an
  existing ledger. Do not claim execution-based verification without running
  the commands.
- Contract validation, reference/example rejection checks, link review, and
  `git diff --check` cover this documentation/tooling change. No runtime
  behavior or database preparation is needed.
- The change introduces no runtime dependencies, API behavior, migrations,
  or changes to local untracked documentation.

## Non-goals

- Generating handlers, clients, schemas, or the OpenAPI contract from Go source.
- Swagger UI, hosted documentation, a docs server, or deployment automation.
- New endpoints, authentication, observability, or ledger functionality.
- Replacing feature specs, claiming production readiness, or operating on
  developer databases while writing documentation.

## Decisions and verification

- Hand-maintained OpenAPI 3.1.0 and public guides at the locations above.
- MIT-licensed `@redocly/cli@2.77.0`, downloaded through npm's cache, without a
  global install or Node dependency in the Go application.
- Spec-focused rules plus explicit unresolved-reference and example checks.
  Telemetry is disabled; no login, hosted validation, code generation, or
  documentation server is used.
- `make lint-openapi` is shared by local development and an independent CI
  job using Node 24. Semantic validation does not establish runtime conformance;
  existing handler/application tests and contract review remain necessary.
- Temporary invalid-reference/example fixtures verify the validator fails
  on errors instead of merely parsing YAML.
- `make docs-openapi` was built with cached tooling in npm offline mode.
  Headless Chromium opened the local file and navigated all five operations
  with external host resolution blocked. Embedded paths match the source YAML;
  viewer assets are inline and generated HTML is ignored.
- The setup walkthrough was reviewed against tracked sources, but not executed
  against a new database as part of this documentation change. No developer
  data, services, migrations, initialization, or private local guides were
  changed.
