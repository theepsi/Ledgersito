# Getting started

Ledgersito is an educational ledger for fictional SITO, **not a production or
real-money system**. It has no authentication or authorization. Keep it on
loopback; do not expose this walkthrough's credentials or API publicly.

Run commands from the repository root. These are instructions for you to run,
not automatic setup: starting the API never starts PostgreSQL, applies
migrations, or initializes the ledger.

## Prerequisites

| Tool | Purpose |
|---|---|
| [Go](https://go.dev/doc/install), matching `go.mod` (`1.27.0`) | Build, run, and test the application. |
| [Docker with Compose](https://docs.docker.com/compose/install/) | Provided local PostgreSQL 17 service; an independently managed PostgreSQL 17 also works. |
| [Goose](https://github.com/pressly/goose#install), `v3.29.0` as in CI | Apply versioned SQL migrations. |
| curl | Send the example HTTP requests. |
| make | Run the convenience targets in the Makefile, if desired. |

Development checks additionally use
[sqlc](https://docs.sqlc.dev/en/stable/overview/install.html) `v1.31.1`,
[golangci-lint](https://golangci-lint.run/docs/welcome/install/) `v2.15.0`,
and Node/npm for the OpenAPI check. These are not API runtime dependencies.
Install tools deliberately using their official instructions; no AUR installer
or automatic project bootstrap is required.

## Environment and database

Create `.env` from the tracked example **only if no local file already exists**:

```sh
if [ ! -e .env ] && [ ! -L .env ]; then
  cp .env.example .env
fi
```

Review the file before loading it: shell sourcing executes its contents. It
is gitignored; do not commit credentials. In each shell running Go commands:

```sh
set -a
. ./.env
set +a
```

| Variable | Meaning |
|---|---|
| `POSTGRES_PORT` | Compose host port, default `5432`; database URLs must match it. |
| `DATABASE_URL` | Ledger used by the API, initialization, reconciliation, and explicitly targeted migrations. |
| `TEST_DATABASE_URL` | Separate disposable database named `ledger_test`; never point it at ledger data. |
| `HTTP_ADDR` | Optional API listen address, default `127.0.0.1:8080`. |
| `TRACE_ENABLED` | Optional local request tracing: `true` enables it; unset, empty, or `false` disables it. |

The example URLs use `ledger:ledger` on `localhost`, with database names
`ledger` and `ledger_test` and `sslmode=disable`. These are local-only example
credentials. Changing `POSTGRES_PORT` also requires updating the URLs. Never
assume an existing `.env` has these values: check the intended database before
running any schema/data command.

**Starts a service and may create the persistent `pgdata` volume:**

```sh
docker compose up -d --wait postgres
docker compose ps
```

Compose binds PostgreSQL to `127.0.0.1`, uses PostgreSQL 17, and checks readiness
with `pg_isready`. On a new volume it creates `ledger`, not `ledger_test`.
Existing volume contents are retained; changing Compose environment values
does not recreate that data.

**Changes schema at `DATABASE_URL`:** inspect migration status, then apply
pending migrations explicitly. Do this before initialization or API startup.

```sh
goose -dir migrations postgres "$DATABASE_URL" status
goose -dir migrations postgres "$DATABASE_URL" up
```

History/replay migrations are forward-only. Do not use `goose down`, database
resets, or volume deletion as routine setup or cleanup. Migration 5 records
posting times for new postings; it deliberately leaves earlier transactions'
timestamps unknown.

**Changes data at `DATABASE_URL` on first initialization:**

```sh
go run ./cmd/initledger
```

This creates one fixed supply of 100,000,000 sititos (1,000,000 Sitos), an origin
at `system-origin`, a reservoir at `system-reservoir`, and the balanced opening
transaction. Repeating the command validates the existing state without adding
funds. It fails explicitly on inconsistent state; do not reset data to bypass
that failure.

Start the API in its own terminal with the environment loaded:

```sh
go run ./cmd/api
```

Startup checks database connectivity; it does not prepare the schema. Keep
the default loopback address. In the following examples:

```sh
BASE_URL=http://127.0.0.1:8080
```

Adjust the URL if you explicitly changed `HTTP_ADDR`.

The process-only health check does not access PostgreSQL. Readiness also
requires the database and migration 5; it does not require ledger
initialization:

```sh
curl -i "$BASE_URL/healthz"
curl -i "$BASE_URL/readyz"
```

Operational counters and latency histograms are available without querying
the database:

```sh
curl -i "$BASE_URL/metrics"
```

They are process-local and reset on API restart. Storage-operation errors
include business rejections as well as database failures; see the
[metrics spec](specs/observability-metrics.md).

## Collect metrics with Prometheus (optional, Linux)

The separate `compose.prometheus.yaml` runs only Prometheus; the normal
`docker compose up` PostgreSQL workflow does not start it. It uses Linux host
networking to scrape the API at `127.0.0.1:8080` without changing the API's
loopback binding. A bridged container would have a different localhost.
Host networking shares the host's network namespace; it is not network
isolation. These instructions target Linux Docker Engine, not Docker Desktop.

The pinned image is `prom/prometheus:v3.13.5`. Prometheus needs no database
credentials, migrations, initialization, tracing changes, or new Go dependency.
Its UI listens only on `127.0.0.1:9090`. If you change `HTTP_ADDR`, explicitly
update the target in `observability/prometheus.yml` to match, keeping it local.

Commands below use the separate `ledgersito-observability` project declared in
the file. Do not merge it with `compose.yaml` or override its project name
through `COMPOSE_PROJECT_NAME` or `-p`. `--env-file /dev/null` avoids loading
the database `.env` for this independent setup.

**Checks the Compose model only; no download or container startup:**

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml config --quiet
```

**Downloads the pinned image**, only when you choose to acquire it:

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml pull prometheus
```

**Runs a temporary validation container, not the Prometheus server.**
The read-only config mount, disabled networking, and explicit entrypoint check
the scrape config without attaching the history volume. `--pull=never` fails
if you have not acquired the image. No native `promtool` installation is needed.

```sh
docker run --rm --pull=never --network=none \
  --mount type=bind,source="$(pwd)/observability/prometheus.yml",target=/etc/prometheus/prometheus.yml,readonly \
  --entrypoint /bin/promtool \
  prom/prometheus:v3.13.5 check config /etc/prometheus/prometheus.yml
```

**Starts Prometheus and creates its dedicated history volume** if absent.
It does not start or restart the API or PostgreSQL. Start the API separately
only when desired, using the existing instructions.

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml up -d --pull never prometheus
docker compose --env-file /dev/null -f compose.prometheus.yaml ps
curl --fail --show-error http://127.0.0.1:9090/-/ready
```

If the readiness request fails, inspect the container logs and retry once it
has finished starting; a running container alone does not prove readiness:

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml logs --tail=50 prometheus
```

Open [target health](http://127.0.0.1:9090/targets) to inspect the `ledgersito`
job, then use the [query UI](http://127.0.0.1:9090/query). After a successful
scrape, this query returns `1`:

```promql
up{job="ledgersito"}
```

`up` describes scrape success, not database readiness or ledger correctness.
The API's `/metrics` does not access PostgreSQL. If the API is unavailable,
the target should report `DOWN` and an attempted scrape produces `up = 0`.
Do not stop the API or PostgreSQL merely to reproduce that state.

To populate a small read-only example while the API is running:

```sh
curl -i http://127.0.0.1:8080/accounts
curl -i http://127.0.0.1:8080/prometheus-learning-not-found
```

The second request intentionally returns `404`; neither request writes ledger
data. Allow at least two scrapes (15 seconds apart) after a label combination
first appears before expecting a rate. Repeat the requests across scrapes
to see activity rather than only an unchanged counter. For completed HTTP
requests per second by status class:

```promql
sum by (status_class) (
  rate(ledgersito_http_requests_total{job="ledgersito"}[5m])
)
```

See the [collection spec](specs/observability-prometheus.md#learning-queries)
for application-only rates, estimated p95 latency, storage-operation rates,
and interpretation of error fractions. An absent series is not automatically
zero, idle workloads can produce undefined ratios/quantiles, and storage
errors include business rejections. Requests and replays are attempts, not
distinct ledger transactions.

Scrapes themselves do not increment the API's HTTP metrics. They still
produce existing request logs and, when tracing is enabled, local HTTP spans.
Prometheus collects neither logs nor spans.

History is held outside the worktree in the dedicated named volume
`ledgersito-observability_prometheus_data`. Retention is 24 hours or 1 GB,
whichever applies first. **1 GB is not a hard total-disk cap:** head/WAL files
and delayed cleanup need additional space. API restarts reset counters but
not collected history; `rate` handles observed resets, not unsampled activity.
Stopping collection creates gaps that are not backfilled.

**Stops and removes only Prometheus's container; keeps its history:**

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml down
```

**Optional, destructive cleanup: also deletes this project's metrics history.**
Run only if you no longer want it. Do not substitute the PostgreSQL Compose
file or run its volume-removal command.

```sh
docker compose --env-file /dev/null -f compose.prometheus.yaml down --volumes
```

The downloaded image remains cached after cleanup. Image removal is a separate
Docker action; this workflow does not prune images or unrelated resources.

## Try local request tracing

Tracing is off by default. To enable it, stop the API if it is already running,
then restart it in a terminal with the existing database environment loaded:

```sh
TRACE_ENABLED=true go run ./cmd/api
```

In another terminal, send these read-only requests:

```sh
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
```

Look in the API terminal's stderr for JSON records with
`"event":"span_completed"`. `/healthz` produces an `http /healthz` root
span. `/readyz` produces a `storage readiness` child followed by an
`http /readyz` root: their `trace_id` values match, and the child's
`parent_span_id` matches the root's `span_id`. Each span includes
`start_time`, `duration_ms`, and an `ok` or `error` outcome. Readiness can
return an error if its database or migration check fails.

For example, the readiness span records look like this (IDs and timings vary):

```json
{"time":"2026-01-01T10:00:00.010Z","level":"INFO","msg":"trace span completed","event":"span_completed","trace_id":"11111111111111111111111111111111","span_id":"2222222222222222","span_kind":"storage","span_name":"storage readiness","start_time":"2026-01-01T10:00:00.001Z","duration_ms":9,"outcome":"ok","parent_span_id":"3333333333333333"}
{"time":"2026-01-01T10:00:00.010Z","level":"INFO","msg":"trace span completed","event":"span_completed","trace_id":"11111111111111111111111111111111","span_id":"3333333333333333","span_kind":"http","span_name":"http /readyz","start_time":"2026-01-01T10:00:00Z","duration_ms":10,"outcome":"ok"}
```

Tracing is sampled for every handled request while enabled, including health
checks and `/metrics`, and may produce substantial stderr output. These local
span records are not OpenTelemetry exports; they are not saved after the
terminal output is lost unless you separately capture it. Span records exclude
caller and ledger values, but existing diagnostic logs are separate; consider
their contents before sharing or saving logs.

To turn tracing off, stop the API and restart it with
`TRACE_ENABLED=false go run ./cmd/api`, or leave `TRACE_ENABLED` unset.
See the [tracing spec](specs/observability-tracing.md) for behavior and limits.

## Create and move fictional funds

These POST requests **change the ledger at the API's `DATABASE_URL`**.
The absolute totals below assume a newly initialized ledger with no transfers.
Existing ledgers keep their previous activity; do not erase it to match examples.

Choose a new key prefix for each new walkthrough. Keep the same prefix and
payload when retrying an uncertain request; changing the key could duplicate
the operation. The readable keys here are teaching examples, not a random-key
generator. Clients should generate and retain random keys. There is no
per-user key isolation because there are no users or authentication.

```sh
RUN_KEY=demo-001

curl -i --request POST "$BASE_URL/accounts" \
  --header 'Content-Type: application/json' \
  --header "Idempotency-Key: ${RUN_KEY}-alice" \
  --data '{"name":"Alice"}'

curl -i --request POST "$BASE_URL/accounts" \
  --header 'Content-Type: application/json' \
  --header "Idempotency-Key: ${RUN_KEY}-bob" \
  --data '{"name":"Bob"}'
```

Both return `201` and one account object directly, with `role: ordinary` and
zero balance. Copy each returned `id` (not the non-unique name) into variables:

```sh
ALICE_ID='paste-Alice-id-here'
BOB_ID='paste-Bob-id-here'
```

Account IDs generated by this API are safe to interpolate in these examples.
Do not send the placeholder values. Fund Alice, then send some funds to Bob:

```sh
curl -i --request POST "$BASE_URL/transfers" \
  --header 'Content-Type: application/json' \
  --header "Idempotency-Key: ${RUN_KEY}-fund-alice" \
  --data "{\"source_account_id\":\"system-reservoir\",\"destination_account_id\":\"$ALICE_ID\",\"amount_sititos\":250}"

curl -i --request POST "$BASE_URL/transfers" \
  --header 'Content-Type: application/json' \
  --header "Idempotency-Key: ${RUN_KEY}-alice-to-bob" \
  --data "{\"source_account_id\":\"$ALICE_ID\",\"destination_account_id\":\"$BOB_ID\",\"amount_sititos\":70}"
```

Success is `201` with a transfer ID, both account IDs, and an amount object;
no balances are returned. Amounts are positive integer sititos, not floating
point, exponent notation, or strings. Origin cannot transfer; the reservoir
can send to ordinary accounts but cannot receive.

**Safe replay check:** rerun the exact 70-sitito request with the same
`${RUN_KEY}-alice-to-bob` key and payload. Expect the original `201` and body,
not another transfer. Later balance changes do not change the stored response.
Changing a committed key's payload returns `422 idempotency_key_reused`.
Account creation and transfer keys have separate scopes.

**Validation check (no ledger writes):**

```sh
curl -i --request POST "$BASE_URL/transfers" \
  --header 'Content-Type: application/json' \
  --header "Idempotency-Key: ${RUN_KEY}-invalid" \
  --data "{\"source_account_id\":\"$ALICE_ID\",\"destination_account_id\":\"$BOB_ID\",\"amount_sititos\":0}"
```

Expect `400` with `{"error":{"code":"invalid_amount","message":"..."}}`.
Only successful responses reserve keys. Failed requests may succeed later if
their cause is corrected. On a timeout or connection loss, retry the same key
and payload: an error does not prove that commit failed.

## Inspect history and analytics

These GET operations are read-only:

```sh
curl -i "$BASE_URL/accounts"
curl -i "$BASE_URL/transactions"
curl -i "$BASE_URL/analytics"

curl -i --get "$BASE_URL/analytics" \
  --data-urlencode 'from=2026-01-01T00:00:00Z' \
  --data-urlencode 'to=2026-02-01T00:00:00Z'
```

Change both bounds to the period you intend to inspect; the example window
does not promise to contain your new transfers. Bounds are RFC 3339 with an
explicit offset and `from < to`. `--data-urlencode` also preserves `+` offsets.

After only the two successful transfers above, lifetime analytics should show:

| Metric | Value |
|---|---|
| Transfer count, excluding opening and replays | `2` |
| Total transfer volume | `"320"` sititos |
| Reservoir distribution count / volume | `1` / `"250"` sititos |
| Ordinary-to-ordinary count / volume | `1` / `"70"` sititos |
| Funds in circulation | `250` sititos |
| Reservoir balance | `99999750` sititos |
| Total supply | `100000000` sititos |
| Untimestamped transfer count / volume | `0` / `"0"` sititos |

Alice holds 180, Bob 70. History contains three transactions, including opening,
in entry-ID order; it does not expose `posted_at`. Account lists include roles
and use an `accounts` envelope; history uses a `transactions` envelope.

Lifetime analytics includes old untimestamped transfers; bounded flows exclude
them and use `from <= posted_at < to`. Timestamp coverage always describes the
whole ledger. `current` always describes the current snapshot, not balances at
the window's end. Flow volumes are decimal **strings** for exact large totals;
balances, supply, and counts are JSON integers. Preserve int64 precision in
clients rather than assuming JavaScript numbers are always exact.

Paths are exact: no trailing slash, encoded aliases, HEAD shortcut, pagination,
or filters beyond analytics bounds. See the [OpenAPI contract](openapi.yaml)
for all five operations and errors.

## Reconcile independently

This command reads `DATABASE_URL` in one consistent, read-only snapshot. It
does not migrate, initialize, or repair:

```sh
go run ./cmd/reconcile
```

The expected result is `Ledger is consistent.` Build an executable when exact
process exit codes matter (`go run` can obscure nonzero program codes):

```sh
go build -o ./reconcile-check ./cmd/reconcile
./reconcile-check
```

Codes are `0` consistent, `1` invariant findings, `2` operational failure, and
`3` cleanly uninitialized. Operational failure means no consistency result was
obtained. Remove the locally built `reconcile-check` executable when finished;
do not commit it.

## Development checks

No database is needed for:

```sh
go test ./...
go vet ./...
gofmt -l $(git ls-files '*.go')
golangci-lint run
sqlc generate
git diff --exit-code -- internal/adapters/postgres/dbsql
```

`gofmt -l` should print nothing; CI explicitly fails if it lists files.
`sqlc generate` rewrites generated files, so the final diff check detects drift;
review differences rather than silently accepting them.

For OpenAPI, use Node 24 (as CI does), or supported Node 22 >=22.12.0, with
npm >=10:

```sh
make lint-openapi
```

The target downloads/runs pinned MIT-licensed `@redocly/cli@2.77.0` and its
dependencies through npm's cache. It does not install globally or add Node to
the Go runtime. Telemetry is disabled in both the command and `redocly.yaml`;
validation is local, with no account, hosted upload, generated docs, or server.
The spec ruleset and explicit reference/example checks reject structural
errors, unresolved references, and invalid examples. This does **not** prove
the API matches the document: handler tests and contract review still matter.
The same command runs in the independent OpenAPI CI job.

### Read the API reference in your browser

`docs/openapi.yaml` is the machine-readable source: it describes paths, request
fields, response schemas, and errors. Tools can validate it and generate a
readable view. It is not application code and does not enforce the Go API's
behavior.

To turn that contract into a browser-readable reference:

```sh
make docs-openapi
```

Open `docs/openapi.html` using your browser's Open File command, or on Linux:

```sh
xdg-open docs/openapi.html
```

This uses the already-approved pinned Redocly tool, validates first, and bundles
the viewer scripts with Google Fonts disabled. After the npm tool is cached,
the HTML can be built and read without a CDN, hosted upload, account, running
API, or documentation server. It is browsable documentation, not a "Try it out"
request console.

The generated HTML is gitignored. Edit `docs/openapi.yaml`, not the HTML, and
rerun `make docs-openapi` after contract changes. Markdown guides remain
separate explanations of setup and architecture; the HTML is simply a rendered
view of the existing API contract.

### Disposable PostgreSQL tests

**Separate, explicit preparation:** Compose does not create `ledger_test`.
With the provided local Compose service, create it once:

```sh
docker compose exec -T postgres createdb -U ledger ledger_test
```

If it already exists, confirm it is the intended disposable test database
instead of deleting/recreating it. Ensure `TEST_DATABASE_URL` points to that
database, not `DATABASE_URL`. The tests verify the database name but cannot
protect valuable data that someone put in a database called `ledger_test`.

**Changes schema only at `TEST_DATABASE_URL`:**

```sh
goose -dir migrations postgres "$TEST_DATABASE_URL" up
```

Run the same database selectors as CI:

```sh
go test -tags=integration -count=1 ./internal/adapters/postgres
go test -tags=integration,initializationcheck -count=1 ./internal/adapters/postgres
go test -tags=integration,migrationcheck -count=1 -run 'Test(NameMigration|PostedAtMigration)' ./internal/adapters/postgres
```

The base integration suite assumes existing migrations; it does not prepare
the database. Initialization, transfer, history, reconciliation, and analytics
checks under `initializationcheck` migrate and initialize unique test schemas,
then clean those schemas up. Migration checks exercise older/newer versions
in isolated transactional schemas. Test fixtures can write, so all selectors
require disposable data and appropriate schema permissions. Do not initialize
the shared test schema yourself.

`make test-integration` runs only the base selector, not all three CI selectors.

## Stop without deleting data

Stop the API with Ctrl-C (SIGINT); it allows active requests a bounded graceful
shutdown. Stop the local database without removing its persistent volume:

```sh
docker compose stop postgres
```

No volume removal or database reset is part of this guide. Continue with the
[architecture guide](architecture.md), [feature specs](specs/README.md), and
[roadmap](ROADMAP.md).
