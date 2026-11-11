# Architecture and ledger correctness

Ledgersito is a learning project using Go and PostgreSQL 17. It is not for
production, real money, or commercial use. Accounts are ledger accounts, not
users; there is no ownership, authentication, or authorization. Resource limits
and idempotency do not replace those controls. No security review or operational
readiness is claimed.

## Boundaries

```text
cmd/api -> HTTP adapter -> application services -> domain rules
                               |
                               v
                     storage interfaces
                               ^
                               |
                   PostgreSQL adapter -> sqlc queries -> PostgreSQL

cmd/initledger -> initialization use case -> PostgreSQL initialization function
cmd/reconcile  -> reconciliation use case -> read-only PostgreSQL checks
```

`cmd/` wires concrete dependencies; application services depend on storage
interfaces, not SQL implementations.

| Location | Responsibility |
|---|---|
| `internal/domain/money` | Exact signed-int64 sititos and checked addition/subtraction. |
| `internal/domain/account` | Identity and normalized names; ordinary-account construction. |
| `internal/domain/transfer` | Amount, endpoint, role, funds, and resulting-balance rules. |
| `internal/application` | Coordinate creation, transfers, initialization, history, analytics, and reconciliation through interfaces. |
| `internal/adapters/httpapi` | Exact routing, strict JSON/media/header validation, deadlines, response encoding, error mapping. |
| `internal/adapters/postgres` | Transaction boundaries, replay persistence, locking, snapshots, query/result mapping. |
| `internal/adapters/postgres/queries` | Hand-maintained SQL, including aggregation and independent invariant checks. |
| `internal/adapters/postgres/dbsql` | sqlc-generated Go; regenerate rather than hand-edit. |
| `migrations` | Goose-managed schema evolution, constraints, triggers, and initialization function. |

The [OpenAPI contract](openapi.yaml) describes HTTP shapes; detailed
[feature specs](specs/README.md) remain the approved behavioral source of truth.
The contract is hand-maintained, not used to generate handlers or served by
the API. CI validates its structure, references, and examples, not runtime
conformance automatically.
`make docs-openapi` renders the same YAML as a local HTML reference; it does
not generate a second contract or change the runtime API.

## Exact amounts and fixed supply

SITO is fictional; one Sito has 100 sititos. Individual balances and entries
use signed int64 sititos, never floating point. Money arithmetic explicitly
rejects overflow. Transfers accept positive integer-token amounts only.
Sufficient representability is not sufficient funds.

Explicit `cmd/initledger` creates exactly 100,000,000 sititos once:

| Role | Initial balance | Policy |
|---|---|---|
| `origin` (`system-origin`) | `-100000000` | Bookkeeping counterpart; never transfers. |
| `reservoir` (`system-reservoir`) | `100000000` | Undistributed supply; only sends to ordinary accounts. |
| `ordinary` | `0` | Can receive and circulate existing funds; cannot overdraw. |

Names are trimmed, non-unique human-readable labels; IDs are identity. New
account and transfer IDs come from `crypto/rand.Text()`, with primary keys as
the final uniqueness safeguard. Callers cannot select a system role.

Initialization locks account writes while validating or establishing the
opening state, records the balanced `system-opening` transaction, and writes
a singleton initialization record. A repeat validates existing state without
issuing again. It rejects unexplained pre-initialization balances/history
instead of guessing how to repair them.

The sum of all balances including origin is zero. Excluding origin, reservoir
plus ordinary balances equals the fixed positive supply. The reservoir is a
simulation policy, not blockchain issuance, asset backing, or a requirement
of double-entry accounting.

## Entries, balances, and database guards

`ledger_transactions` groups immutable signed `entries`. A transfer records
one negative source entry and one equal positive destination entry. Its sum
is zero; the opening transaction follows the same balanced principle.
`accounts.balance` is a cached balance updated atomically with transfer entries.
History explains balances; the cache makes current-balance checks convenient.

Database constraints and triggers enforce, among other rules:

- Nonnegative non-origin balances, fixed origin balance, immutable IDs/roles,
  and at most one account for each system role.
- Nonzero entries tied to existing accounts and unposted transactions;
  no later entry can be appended to already-posted history.
- No entry updates/deletes/truncation; no posted-transaction changes or
  transaction-table truncation.
- A deferred constraint trigger requiring posted transactions to have at
  least two entries summing to zero at transaction completion.
- No origin participation or positive reservoir entry outside opening.
  Initialization metadata is protected against mutation.

These are defense-in-depth guards, not a proof against arbitrary privileged
SQL or every conceivable corrupt state. The application controls the complete
write protocol; reconciliation independently detects discrepancies, including
cache/history mismatches.

## Atomicity, concurrency, and retries

The PostgreSQL transfer adapter uses a `READ COMMITTED` transaction:

1. Reserve the transfer-scoped idempotency key. A conflicting reservation
   waits; a later statement sees the committed original response.
2. Resolve a committed replay or payload conflict before mutable business
   checks. For a new operation, coordinate with initialization using a
   `ROW EXCLUSIVE` accounts table lock.
3. Check initialization and lock both endpoint rows using `FOR UPDATE`, in
   ascending byte-ordered ID order regardless of transfer direction.
4. Validate roles, available funds, and checked resulting balances. Record
   the transaction, both entries, both balances, and posted state.
5. Store the successful `201` response and commit everything together.

The table lock allows ordinary transfers/account creation to proceed
concurrently; it is not a global transfer mutex. Consistent row-lock order
avoids opposite-direction account-lock deadlocks. Locking balances before
calculating prevents concurrent overspending. A failed operation rolls back
partial entries, balances, and replay reservation together.

Creation uses its own key namespace and commits account plus successful
response atomically. Keys are global across callers within each operation
scope because there is no authentication. Creation identity is the normalized
name; transfer identity is both IDs and the integer amount. Only successful
responses persist. A failed insufficient-funds attempt can succeed on a later
same-key retry after funding.

Same-key/same-payload success replays the original status and response bytes,
not a reconstruction from current state. Keys do not expire. Old creation
responses may omit `role`; current listings always include it. A committed
key reused for a different payload yields `422 idempotency_key_reused`.

HTTP operations have five-second application/database deadlines, including
key/lock waits. Timeout or lost connection during commit does not prove
rollback. Retry the same key and payload to recover the outcome; using a new
key risks moving funds again.

## Schema evolution and posting time

Goose migrations are explicit; the API does not run them. Ledger-history and
replay migrations refuse destructive rollback. Change schema forward instead
of editing applied migrations or resetting a ledger to bypass migration issues.
`sqlc generate` produces typed query code; CI rejects generated-code drift.

Migration 5 adds nullable `posted_at TIMESTAMPTZ`. PostgreSQL observes
`clock_timestamp()` when marking a transaction posted (including a posted
insert); unposted transactions have no posting time. This is database
wall-clock metadata, not an exact commit timestamp or strict ordering.
Transactions already posted before the migration retain `NULL`: their true
times are unknown, not invented during migration.

`GET /transactions` exposes no timestamp. Transactions are ordered by their
smallest entry ID, with each transaction's entries ordered by ID. IDs may be
allocated before commit, so this is deterministic allocation order rather
than a chronological commit timeline.

## Analytics versus reconciliation

Both use PostgreSQL `REPEATABLE READ`, read-only transactions, so concurrent
commits cannot mix different snapshots within a report. Neither initializes,
migrates, or repairs the ledger.

**Analytics answers what happened to funds.** It assumes ledger invariants
hold. The aggregate query counts each posted non-opening transfer once using
negative source entries, separating reservoir distributions from subsequent
ordinary-to-ordinary movement.

Flows are lifetime or half-open `from <= posted_at < to`. Lifetime includes
legacy unknown times; bounded flows exclude them. Whole-ledger unknown-time
coverage is reported in both modes. `current` always reports current snapshot
stock, not historical balances at the window's end.

Cumulative volume uses PostgreSQL `numeric` aggregation and decimal strings
in JSON: repeated circulation can exceed int64 even with a small fixed supply.
Current balances and supply remain int64 JSON numbers. Clients must preserve
precision for integers outside JavaScript's exact number range.

A distribution of 250 then circulation of 70 yields total flow 320, while
ordinary accounts hold only 250. Reservoir distribution and current ordinary
stock happen to agree under today's one-way reservoir policy; they are not
two separate portions to add together.

**Reconciliation answers whether persisted invariants agree.** Independent
SQL checks compare cached balances with entry sums, transaction balance and
posted state, system-account rules, opening history, and supply. Findings are
diagnostics, not automatic repair. An operational failure reports that no
consistency result is available, not an empty success.

Analytics success and HTTP metrics are not a substitute for reconciliation.
See the [analytics](specs/ledger-analytics.md) and
[reconciliation](specs/reconciliation.md) specs for details.

## Scope and limitations

There are no users, permissions, fees, extra issuance, currency conversion,
pagination, hosted API docs, dashboards, outbox, or deployment automation.
Do not treat this architecture as financial, security, or production guidance.
Future candidates require separate specifications and approval; see the
[roadmap](ROADMAP.md). For commands and safe database preparation, use
[getting started](getting-started.md).
