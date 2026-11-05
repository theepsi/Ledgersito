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
- The user has also tried the account/transfer API manually.

## Next: verify reconciliation against PostgreSQL

The [reconciliation spec](specs/reconciliation.md) is approved and implemented:
an explicit read-only CLI with text reports, distinct exit codes, and one
REPEATABLE READ snapshot. It checks cached balances, posted/balanced history,
fixed supply, system identities/metadata, opening history, and reservoir policy.
It does not call initialization or change its behavior.

Real-PostgreSQL execution remains pending at the user's request; no test
database connection is configured in the session. Once the user provides
`TEST_DATABASE_URL` for prepared `ledger_test` and approves schema setup, run
the focused reconciliation checks documented in the spec. Valid/corrupt-state,
read-only, and controlled concurrent-write scenarios use isolated test schemas
only. Do not mark the feature database-verified until those checks pass.

## Later candidates, not yet approved

1. CI for existing formatting, lint, unit, SQL-generation, and database checks.
2. Read-only transaction/history queries to explore entries and transfers.
3. Ledger analytics: read-only statistics for transfer counts and amounts,
   reservoir distribution, and funds in circulation. Define units, time windows,
   required timestamps, and snapshot semantics before implementation; distinguish
   reservoir distribution from subsequent circulation to avoid double counting.
4. OpenAPI documentation and self-contained public setup/design documentation.
5. Transactional outbox and repeated-delivery safety, if still useful for learning.
6. Operational observability: health/readiness checks, structured logs, request
   and database latency/error metrics, and tracing as separately agreed specs.
   Explore an external collection/dashboard stack, then use a controlled failure
   to learn diagnosis and useful alerting. Keep account IDs and idempotency keys
   out of metric labels; avoid sensitive data in telemetry.
7. Final review of learning goals, invariants, limitations, and completion criteria.

Ledger analytics answers what happened to funds; operational observability
answers how the service behaves. Reconciliation independently checks persisted
correctness: neither dashboards nor successful-request metrics replace it.
Build small read-only queries and instrumentation first, then choose external
tools only when their learning purpose and setup are agreed.

Ordering and scope remain open. No users/authentication, extra issuance, fees,
or real-world deployment requirements are implied.

## Working agreement for future sessions

- Explain concepts and tradeoffs; agree on one clear spec at a time.
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
