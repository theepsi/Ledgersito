# Go Payments Ledger: Project Startup Guide

A small, public Go service whose only purpose is to **prove you think about financial integrity**: idempotency, consistent data, exact money, safe retries, concurrency, and reconciliation. Keep it small and well tested. Quality beats size.

Name: `ledgersito` (a playful Spanish diminutive of "ledger"; the name is the only joke, keep code, packages and docs professional).

Suggested one-line tagline for the repo description and README: *"ledgersito: a tiny double-entry ledger in Go, for learning and games."*

---

## 1. Goals (what the repo must demonstrate)

| Clovr requirement | What the repo shows |
|---|---|
| Idempotency | Every write endpoint requires an `Idempotency-Key`; retries never double-apply |
| Data consistency | Double-entry ledger, all writes inside one DB transaction, DB constraints as a safety net |
| Precise money | Integers in minor units (cents or satoshis), never floats |
| Retries / concurrency | Tests that fire parallel requests at the same account and the same key |
| Partial failures | Outbox pattern: state change and event are written atomically |
| Traceable and reconcilable | Immutable entries, trace IDs in logs, a reconciliation job that finds mismatches |
| Observability | Structured logs, OpenTelemetry traces, Prometheus metrics, one Grafana dashboard |
| Security mindset | Short `THREAT_MODEL.md`, input validation, no secrets in the repo |
| AI-powered engineering | Built spec-driven with TDD; specs live in `docs/specs/` and tests are written before code, so AI output is always checked against them |

**Non-goals:** UI, auth providers, real payment rails, microservices. One service, one Postgres.

---

## 2. Stack

- **Go** (latest stable release)
- **net/http** with the standard library router (Go 1.22+ supports method and path patterns), or `chi` if you prefer
- **PostgreSQL** (Docker Compose)
- **pgx** for the DB driver and **sqlc** for typed queries
- **goose** (or golang-migrate) for migrations
- **log/slog** for structured logs
- **OpenTelemetry** (traces) and **Prometheus client** (metrics)
- **testcontainers-go** for real-Postgres integration tests
- **golangci-lint** and **GitHub Actions** for CI

---

## 2b. Prerequisites (CachyOS / Arch, official repos only)

You don't need Go or Postgres installed beyond this: Go on the host, Postgres in Docker.

```bash
# Go toolchain, Docker and Compose
sudo pacman -S go docker docker-compose

# start Docker now and on boot
sudo systemctl enable --now docker.service

# run docker without sudo (log out and back in afterwards)
sudo usermod -aG docker $USER

# make Go-installed tools available in your shell (add to your shell config)
export PATH="$PATH:$(go env GOPATH)/bin"

# dev tools, installed through Go itself (no AUR needed)
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
# golangci-lint: check `pacman -Ss golangci-lint`, otherwise use its official install script
```

Notes:
- Membership in the `docker` group is effectively root access on your machine. If that bothers you, Podman works too (`podman` plus `podman-compose`), but testcontainers needs extra socket setup, so Docker is simpler here.
- Verify with `go version` and `docker run --rm hello-world`.

### `docker-compose.yml` (Postgres only)

```yaml
services:
  postgres:
    image: postgres:17
    environment:
      POSTGRES_USER: ledger
      POSTGRES_PASSWORD: ledger
      POSTGRES_DB: ledger
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ledger"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
```

Local-only credentials like these are fine for development. Never reuse them anywhere real, and put real settings in environment variables (`DATABASE_URL`), not in git.

Two ways to use the database:
- **Dev server:** `docker compose up -d`, then run migrations with goose against `postgres://ledger:ledger@localhost:5432/ledger?sslmode=disable`.
- **Tests:** testcontainers starts its own throwaway Postgres per test run, so tests never touch your dev data and need no compose file running (only the Docker daemon).

---

## 3. Setup commands

```bash
mkdir ledgersito && cd ledgersito
git init
go mod init github.com/<your-user>/ledgersito

mkdir -p cmd/api cmd/reconcile internal/{ledger,idempotency,outbox,httpapi,money,telemetry} migrations docs/specs
touch README.md docs/DESIGN.md docs/THREAT_MODEL.md Makefile docker-compose.yml

go get github.com/jackc/pgx/v5
go get github.com/google/uuid
go get github.com/prometheus/client_golang/prometheus
go get go.opentelemetry.io/otel
go get github.com/testcontainers/testcontainers-go
```

Then write your first spec from section 7, before any code.

### Suggested layout

```
cmd/api/            HTTP server entrypoint
cmd/reconcile/      reconciliation job entrypoint
internal/money/     Money type (int64 minor units + currency)
internal/ledger/    accounts, transfers, balance logic
internal/idempotency/  key storage and replay logic
internal/outbox/    outbox table + publisher loop
internal/httpapi/   handlers, validation, middleware
internal/telemetry/ logging, tracing, metrics setup
migrations/         SQL migrations
docs/               DESIGN.md, THREAT_MODEL.md
docs/specs/         one short spec per feature (behaviour, invariants, edge cases)
```

---

## 4. Core design decisions (write the reasoning in `docs/DESIGN.md`)

### Money
- Store as `BIGINT` minor units plus a currency code. Never `float`, and avoid `NUMERIC` unless you have a reason.
- Wrap in a `Money` type that rejects mixed currencies and checks overflow.
- Rule: amounts are always positive in the API; direction comes from debit/credit.

### Double-entry ledger
Every transfer creates one `transaction` and at least two `entries`. The entries of a transaction must **sum to zero**.

Suggested schema:

```sql
CREATE TABLE accounts (
  id          UUID PRIMARY KEY,
  currency    TEXT NOT NULL,
  balance     BIGINT NOT NULL DEFAULT 0,          -- cached, updated in the same tx
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE transactions (
  id           UUID PRIMARY KEY,
  reference    TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE entries (
  id             BIGSERIAL PRIMARY KEY,
  transaction_id UUID NOT NULL REFERENCES transactions(id),
  account_id     UUID NOT NULL REFERENCES accounts(id),
  amount         BIGINT NOT NULL,                 -- signed: debit negative, credit positive
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE idempotency_keys (
  key            TEXT PRIMARY KEY,
  request_hash   TEXT NOT NULL,                   -- same key + different body = 422
  status_code    INT,
  response_body  JSONB,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
  id            BIGSERIAL PRIMARY KEY,
  topic         TEXT NOT NULL,
  payload       JSONB NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at  TIMESTAMPTZ
);
```

Rules:
- Entries are **append-only**. Corrections are new compensating transactions, never updates or deletes.
- Add a DB-level protection against negative balances if the account type requires it (for example a `CHECK (balance >= 0)` on non-overdraft accounts).
- A sanity query must always return zero: `SELECT SUM(amount) FROM entries;`

### Idempotency
Flow for `POST /transfers`:
1. Require header `Idempotency-Key`.
2. In one DB transaction: insert the key row. If it already exists:
   - same request hash and finished: return the stored response
   - same request hash and still in progress: return `409` (or wait)
   - different hash: return `422`
3. Apply the transfer.
4. Store the response in the key row, commit.

A crash at any point must leave either "nothing happened" or "everything happened and the response is stored".

### Concurrency
- Lock the accounts involved in a **consistent order** (for example sorted by ID) with `SELECT ... FOR UPDATE` to avoid deadlocks.
- Alternatively use `SERIALIZABLE` and retry on serialization failure. Pick one and document why.

### Partial failures
- Never publish an event directly from the handler. Write to `outbox` in the same DB transaction, and let a separate loop publish and mark `published_at`. Consumers must be idempotent too (at-least-once delivery).

### Reconciliation
`cmd/reconcile` checks:
- the sum of all entries is zero
- each account's cached `balance` equals the sum of its entries
- every transaction's entries sum to zero
- (bonus) compare against a fake "external provider" CSV and report missing or extra items

It exits non-zero and logs structured findings when it detects a mismatch.

---

## 5. Milestones

1. **Skeleton:** Docker Compose with Postgres, migrations, `/healthz`, structured logs, CI that runs lint and tests.
2. **Money and accounts:** `Money` type with unit tests; create account endpoint.
3. **Transfers:** double-entry transfer inside a single DB transaction, with validation.
4. **Idempotency:** key table and replay logic, with tests for replay, conflict, and different-body cases.
5. **Concurrency tests:** 100 parallel transfers on shared accounts; assert final balances and no deadlocks; 50 parallel requests with the same key; assert exactly one transfer.
6. **Failure tests:** inject a crash between steps (for example a failing hook) and assert no partial state.
7. **Outbox:** atomic event write plus publisher loop; test duplicate delivery safety.
8. **Reconciliation job:** with a test that deliberately corrupts a balance and expects detection.
9. **Observability:** OpenTelemetry traces across handler → DB, Prometheus metrics (transfers total, failures, latency, idempotent replays), one Grafana dashboard JSON.
10. **Docs:** `DESIGN.md`, `THREAT_MODEL.md`, README with a diagram and "what happens if the server crashes here" table.

Aim for a first public version after milestones 1 to 5. The rest can follow.

---

## 6. Acceptance tests to be proud of

- Same `Idempotency-Key` sent 50 times concurrently results in exactly one transfer.
- 1,000 random transfers across 10 accounts keep `SUM(entries) = 0` and every account balance equals its entry sum.
- Killing the process mid-transfer never leaves unbalanced entries.
- The same key with a different body is rejected.
- Floats never appear in the code (add a lint rule or a grep check in CI).
- The reconciliation job catches an intentionally corrupted balance.

---

## 7. Working method: spec-driven + TDD

For every milestone in section 5:

1. **Spec first.** Write a short `docs/specs/<feature>.md`: behaviour, invariants (for example "entries of a transaction sum to zero"), error cases, and what must never happen.
2. **Red.** Turn each spec statement into a failing test (unit test, or integration test with testcontainers against real Postgres).
3. **Green.** Write the smallest code that passes. If you use an AI assistant here, give it the spec and the failing tests, and accept nothing the tests don't cover.
4. **Refactor.** Clean up with the tests as a safety net.
5. **Commit per step** (spec, red, green) so the history shows the method.

Invariant-style specs fit this project especially well: the ledger rules ("sum is zero", "same key means one effect", "balance equals sum of entries") are exactly what the tests should assert.

### First spec to write (seed for `docs/specs/ledger-core.md`)

> Create a Go service `ledgersito` that exposes a small HTTP API for a double-entry ledger backed by PostgreSQL. Requirements: money stored as int64 minor units with currency; accounts; transfers that create a transaction with balanced entries in a single DB transaction; mandatory `Idempotency-Key` header with request-hash conflict detection and stored responses; consistent lock ordering to avoid deadlocks; append-only entries; a transactional outbox with a separate publisher loop; a reconciliation command that verifies ledger invariants; structured logging with trace IDs, OpenTelemetry tracing, and Prometheus metrics. Include integration tests using testcontainers covering concurrency, idempotent retries and crash safety. Non-goals: authentication, UI, real payment rails.

Tip: mention in the README that you built it spec-driven with TDD (and with an AI assistant, if you use one), and say how you reviewed the output (tests first, invariants checked, manual review of the SQL). This directly supports the "AI-powered engineering with judgment" point.

---

## 8. Threat model starter (`docs/THREAT_MODEL.md`)

Cover briefly:
- **Replay and duplicate requests:** idempotency keys, key scoping per client
- **Idempotency abuse:** key length limit, expiry policy, rate limiting
- **Integer overflow and negative amounts:** validation at the API and DB level
- **Race conditions:** lock ordering and tests
- **Tampering with history:** append-only entries, no update or delete privileges for the app DB role
- **Secrets:** environment variables only, nothing in git
- **Audit trail:** every entry linked to a transaction, reference, and trace ID

---

## 9. Optional extras if you have time

- Add a tiny **Lightning flavour**: model an invoice that moves from `pending` to `settled` or `expired`, with a state machine and idempotent settlement callbacks. It connects the repo directly to Clovr's domain.
- Property-based test (with `testing/quick` or `rapid`) asserting invariants over random operation sequences.
- A `docs/RUNBOOK.md`: how to investigate a mismatch found by reconciliation.

---

## 10. Definition of done for the first public version

- [ ] CI is green (lint and tests)
- [ ] README has the problem, design, run instructions, and a failure-scenarios table
- [ ] `DESIGN.md` explains money, idempotency, locking, outbox, and reconciliation decisions
- [ ] Concurrency, idempotency, and crash tests pass
- [ ] Reconciliation job works and is tested
- [ ] Link added to your CV and cover letter
- [ ] README starts with the disclaimer block from section 11
- [ ] `LICENSE` file added (MIT or Apache-2.0, both include the no-warranty clause)

---

## 11. Publishing as a public learning repo

Decisions already taken:
- Public repo on GitHub (optionally mirrored from your own Gitea).
- Purpose: **learning and fun**, aimed at games and apps with virtual or non-real money.
- **No support or maintenance**, no security-reporting process (no `SECURITY.md`). Anyone is welcome to fork.
- Optional: turn off the GitHub Issues tab, or state that issues and PRs may go unanswered.
- Keep the repo standalone: don't mention Clovr or Elenpay inside it; link the repo from your application instead.
- Keep git history in small commits (spec, failing test, passing code) to show the TDD method.
- Only local dev credentials (the `docker-compose.yml` ones) may be in git. Real settings go in environment variables.

### README disclaimer (paste at the very top of `README.md`)

```markdown
## ⚠️ Read this first

**ledgersito is a learning and fun project, not a production system.**

- It is a deliberately **non-secure** reference implementation of ledger correctness
  (double-entry, idempotency, exact money, outbox, reconciliation).
- It is fine for **learning, games, and small apps that use virtual or non-real money**.
- It has **no authentication or authorization, no rate limits, and has had no security review.**
  If you plan to use anything like it for real users or real money, you need to add all of
  that yourself, plus your own review, testing, and legal/compliance checks.
- **No support or maintenance.** I don't plan to answer issues or review pull requests.
  You are very welcome to **fork it** and do whatever you like with it.
- Provided "as is", without warranty (see LICENSE).
```

### README outline to build around it

1. Disclaimer block above
2. What it is, in two sentences, plus a small diagram of the transfer flow
3. Quick start: `docker compose up -d`, run migrations, run the API, example `curl` calls
4. Guarantees it demonstrates (money as integers, double-entry, idempotency, outbox, reconciliation)
5. "What happens if the server crashes here" table (failure scenarios)
6. How to run tests (needs the Docker daemon running, since testcontainers starts Postgres)
7. Link to `docs/DESIGN.md`, `docs/THREAT_MODEL.md` and `docs/specs/`
8. "Before using this for anything real" list: auth and authorization, rate limits, hot-account handling, backups, monitoring, key and outbox retention, security review
9. License

### Note for working with Copilot at your PC

Give it the relevant `docs/specs/*.md` file plus the failing tests as context for each step, and keep to the loop in section 7: spec, red, green, refactor. Review every generated SQL query and every lock or transaction boundary by hand, since those are where ledger bugs hide.
