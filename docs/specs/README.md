# Feature specifications

Agree on one feature spec at a time before implementing behavior.

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
  normalized request identity, and atomicity rules. Not implemented; HTTP
  details and the nickname migration remain to be specified.
