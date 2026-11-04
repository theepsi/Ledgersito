# Accounts

The original scope below is implemented. [Account Names](account-names.md)
extends creation to require a name and supersedes its earlier exclusion here.

## Purpose and scope

Define the domain rules for creating an account: a stable identity and an
initial balance in SITO. Keep these rules independent of ID generation,
persistence, and HTTP.

An account identifies a place where ledger entries will be recorded. Its
balance will eventually reflect those entries; creating an account does not
create funds.

## Identity

- Creation receives an ID as a string; the domain does not generate it.
- Reject the empty string and any ID containing Unicode whitespace, including
  spaces, tabs, and newlines.
- Preserve accepted IDs exactly. Do not trim whitespace or change case.
- IDs are case-sensitive: `account-A` and `account-a` are distinct identifiers.
- An account's ID cannot be changed after creation.
- No UUID format or length limit is imposed by this domain spec.
- Identity uniqueness across stored accounts belongs to a later persistence
  spec. Constructing a domain value does not reserve an ID or detect duplicates.

## Creation and balance

- Successful creation returns an account with the supplied ID and a Money
  balance of exactly zero sititos in SITO.
- Callers can read the ID and Money balance.
- Creation does not accept an opening balance or currency.
- The account exposes no operation for setting or changing its balance in
  this scope. Reading a balance does not give callers a way to mutate it.
- An account has no name, owner, timestamps, or other metadata in this scope.
- Accounts with different IDs remain distinct even if their balances match.

## Error behavior

- Creation returns an account and an error.
- Valid IDs produce a nil error and an account satisfying the rules above.
- Invalid IDs produce a non-nil validation error, not a panic.
- On error, the returned account is not a successful creation result and
  must be ignored. Its contents are not part of this contract.

## Acceptance examples

| Supplied ID | Expected outcome |
|---|---|
| `account-1` | ID preserved, zero sititos, no error |
| `account-A` and `account-a` | Both accepted and preserved as different IDs |
| Empty string | Validation error |
| A single space | Validation error |
| Leading or trailing space around `account-1` | Validation error; no trimming |
| `account 1` | Validation error |
| An ID containing a tab or newline | Validation error |
| An ID containing a non-breaking space (U+00A0) | Validation error |
| `account-1` constructed twice | Both succeed locally; no uniqueness lookup |

Focused domain unit tests should verify creation, validation, and exact
identity preservation. They require neither Docker nor PostgreSQL.

## Non-goals

- Application services, repository interfaces, and ID generation.
- Database schema, persistence, duplicate detection, and HTTP endpoints.
- Adding funds, transfers, entries, or arbitrary balance updates.
- Overdraft and negative-balance permissions. Money supports negative values,
  but that does not decide which accounts may have negative balances.
- Authentication, ownership, naming, and lifecycle operations.

## Next boundaries

Later specs will define how the application generates unique IDs and persists
accounts, and how balanced ledger transactions change account balances.
This spec does not yet introduce an application use case because there is no
external operation to coordinate.
