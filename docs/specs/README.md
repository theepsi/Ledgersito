# Feature specifications

For each feature, create a draft spec first. Review and revise it with the user,
then wait for their explicit approval before implementation; discussion or
general enthusiasm is not approval. Keep the spec as the source of truth as the
feature is implemented.

Each spec should state:

- The concept being learned and the scope.
- Observable behavior and invariants.
- Inputs, outputs, validation, and error cases.
- Examples and acceptance tests, including relevant boundaries.
- Explicit non-goals and unresolved decisions.

Then follow red, green, refactor: run a focused failing test, implement the
smallest correct behavior, and refactor with passing tests.

## Specifications

- [Money](money.md): exact signed SITO amounts, addition, subtraction, and
  overflow errors. Implemented with domain unit tests.
- [Accounts](accounts.md): supplied IDs, identity validation, and zero SITO
  opening balances. Implemented with domain unit tests.
- [Create Account](create-account.md): application coordination of ID generation,
  domain construction, and create-only storage. Implemented with application
  unit tests; concrete generators and storage adapters remain out of scope.
- [PostgreSQL Account Storage](postgres-accounts.md): migrations, create-only
  persistence, conflict handling, and opt-in database tests. Implemented and
  verified against PostgreSQL, including concurrent duplicate creation.
- [Account Creation Idempotency](account-idempotency.md): agreed replay,
  normalized request identity, and atomicity rules. Implemented and verified
  against PostgreSQL; old response bodies remain unchanged when roles are added.
- [Account Names](account-names.md): non-unique human-readable labels,
  Unicode validation, and trimming. Implemented and verified.
- [List Accounts](list-accounts.md): all accounts ordered by name then ID,
  with explicit balance units and roles. Implemented and verified.
- [Accounts HTTP API](accounts-http.md): agreed GET/POST response shapes,
  strict POST JSON validation, errors, limits, and server wiring. Implemented
  and verified; new creation responses expose the ordinary role.
- [Ledger Initialization](ledger-initialization.md): fixed supply, origin and
  reservoir roles, and one-time balanced initialization. Implemented and
  verified against PostgreSQL, including isolated initialization checks.
- [Transfers](transfers.md): approved core movement, atomicity, concurrency,
  distribution-only reservoir, retry rules, HTTP contract, and acceptance
  scenarios. Implemented and verified against PostgreSQL.
- [Ledger Reconciliation](reconciliation.md): approved read-only verification
  of ledger balances, history, fixed supply, and system-account invariants.
  Implemented and verified against PostgreSQL.
- [Transaction History](transaction-history.md): approved read-only listing of
  posted transactions and signed entries, with stable entry-ID ordering and
  database posting timestamps for new transactions.
- [Ledger Analytics](ledger-analytics.md): approved and implemented read-only
  lifetime and time-window aggregates distinguishing transfer activity from
  current funds, including exact volumes and legacy timestamp coverage.
- [API and Public Documentation](public-documentation.md): approved and
  implemented hand-maintained OpenAPI 3.1 contract, tracked public setup/design
  guides, and matching local/CI reference and example validation.
- [Continuous Integration](ci.md): approved automation for existing quality,
  generation, unit, and PostgreSQL integration checks.
