# Learning roadmap

This is an educational Go/PostgreSQL ledger, not a production or real-money
system. Feature specifications in [specs](specs/README.md) are the source of
truth for approved behavior. This roadmap is a handoff, not approval to
implement every candidate below.

## Completed foundation

- Exact signed int64 sititos with checked Money arithmetic.
- Named ordinary accounts with zero opening balances.
- PostgreSQL storage, goose migrations, and sqlc-generated queries.
- POST /accounts with atomic successful-response idempotency; GET /accounts.
- Explicit one-time initialization of 100,000,000 sititos, with equal negative
  origin and positive reservoir opening entries.
- POST /transfers with balanced immutable entries, atomic balance updates,
  successful-response replay, ordered account locking, and no overspending.
- A distribution-only reservoir: it sends to ordinary accounts but cannot
  receive transfers. Ordinary accounts circulate funds; origin never transfers.
- Unit/HTTP tests and real-PostgreSQL integration checks, including concurrency,
  rollback, replay, and timeouts. Migration 4 and transfer checks passed.
- Read-only ledger reconciliation CLI checks balances, transaction history,
  fixed supply, and system invariants in one consistent snapshot; focused
  PostgreSQL acceptance tests pass, and the existing local ledger reconciled
  successfully.
- Read-only `GET /transactions` lists posted transactions and signed entries in
  deterministic entry-ID order; its isolated PostgreSQL integration check
  passed. Migration 5 records `posted_at` for newly posted transactions;
  existing transactions retain unknown timestamps. The API remains ordered by
  entry ID; see the [transaction history spec](specs/transaction-history.md).
- Read-only `GET /analytics` reports transfer flows and current stock, supports
  optional `posted_at` windows, and reports legacy unknown timestamps. Exact
  aggregate volumes and snapshot behavior are covered by PostgreSQL
  integration checks; see the [analytics spec](specs/ledger-analytics.md).
- GitHub Actions CI runs Go formatting, unit tests, vet, lint, sqlc generation
  consistency, and PostgreSQL integration, initialization, and migration checks
  using Node 24-compatible actions. See the [CI spec](specs/ci.md).
- The user has also tried the account/transfer API manually.

## Pending candidates and suggested priorities

These are recommendations for sequencing, not approval to implement. Draft and
review one spec at a time before implementation.

1. **P2 — API and public documentation:** Add OpenAPI documentation and
   self-contained public setup/design documentation. The local
   `docs/DESIGN.md` and `docs/SETUP.md` remain intentionally untracked; keep
   those copies untouched and settle the public-document scope in its spec.
2. **P3 — Operational observability:** Add separately specified health/readiness
   checks, structured logs, request/database latency and error metrics, and
   tracing. Start with small instrumentation; consider an external
   collection/dashboard stack only after agreeing on its learning purpose and
   setup. Keep account IDs and idempotency keys out of metric labels and avoid
   sensitive telemetry.
3. **P4 — Transactional outbox:** Explore repeated-delivery safety only if it
   still serves a specific learning goal; defer it until a concrete event
   delivery use case is agreed.
4. **P5 — Final learning review:** Review learning goals, invariants,
   limitations, and completion criteria after selecting or completing the
   desired candidates. Move this earlier if the goal is to decide whether any
   further feature work is worthwhile.

Ledger analytics answers what happened to funds; operational observability
answers how the service behaves. Reconciliation independently checks persisted
correctness: neither dashboards nor successful-request metrics replace it.
Build small read-only queries and instrumentation first, then choose external
tools only when their learning purpose and setup are agreed.

The priorities are provisional and scope remains open. No
users/authentication, extra issuance, fees, or real-world deployment
requirements are implied.

## Working agreement for future sessions

- Explain concepts and tradeoffs; write each feature spec as a draft in
  `docs/specs/`, review and revise it with the user, then wait for their explicit
  approval before implementation. Do not infer approval from general enthusiasm.
  Work on one clear spec at a time.
- Follow spec-first TDD: focused failing tests, minimal implementation, then
  refactor and run relevant checks. Keep code professional and comments sparse.
- The user controls installations, Docker/services, database preparation,
  migrations, initialization, commits, and pushes. Do not execute those without
  explicit approval; provide explained commands. Tests can run when their
  prerequisites and any schema-setup permission are already available.
- Do not install from the AUR.
- Keep docs/DESIGN.md, docs/SETUP.md, and go-ledger-project-startup.md local and
  untracked. Do not stage, delete, or add ignore rules for them.
- Do not edit applied migrations or reset ledger data to work around failures.
- Standard integration tests require ledger_test and never prepare it.
  The explicit integration,initializationcheck selector permits migrations and
  initialization in unique test schemas, followed by targeted schema cleanup.
- A new session should read README.md, this roadmap, and the relevant specs,
  then inspect the current worktree before making changes.
