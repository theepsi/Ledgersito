# Account Names

## Status and purpose

Agreed extension to the existing account domain and creation specs.
Implemented; migration and real-database verification are pending.

Accounts are ledger accounts, not users, registrations, or login identities.
A name is a human-readable label for learning and recognizing accounts.
It does not establish ownership or identify a person.

## Rules

- Require a string `name` at account creation.
- Require valid Unicode text. Trim surrounding Unicode whitespace before
  validating length and storing the name.
- After trimming, require 1-100 Unicode code points, not bytes or visually
  perceived characters.
- Reject remaining Unicode control characters, including embedded tabs and
  newlines.
- Preserve capitalization, internal spaces, and other accepted characters.
- Do not lowercase, collapse internal whitespace, or apply Unicode
  normalization.
- Names are not unique. Distinct accounts can both be named `Savings`.
- The generated account ID remains the stable identity.
- Renaming is outside this scope.

For length and control-character rules, use Go's Unicode rune semantics and
`unicode.IsControl`; trimming follows `strings.TrimSpace`.

## Idempotency identity

Compare the validated, trimmed name. JSON formatting does not matter.
`" Savings "` and `"Savings"` are equivalent payloads; `"Savings"` and
`"savings"` are different. Different internal spacing and different Unicode
code-point sequences remain different, even if they look similar.

Different keys with the same name can create separate accounts. A name is not
a substitute for an idempotency key.

## Acceptance examples

| Supplied name | Expected outcome |
|---|---|
| `Savings` | Preserve exactly |
| ` Savings ` | Store `Savings` |
| `Dragon's savings` | Preserve capitalization and internal space |
| `Game  rewards` | Preserve both internal spaces |
| Non-ASCII text, such as `Caf\u00e9` interpreted as Unicode text | Accept and preserve |
| Empty or whitespace-only string | Validation error |
| 100 Unicode code points after trimming | Accept |
| 101 Unicode code points after trimming | Validation error |
| Embedded tab, newline, or NUL | Validation error |
| Two creations named `Savings` with different keys | Distinct accounts |

This extension supersedes the earlier account specs' exclusion of account
names. It does not change ID validation, opening balances, or Money rules.

## Migration of existing accounts

- Add a new versioned migration; do not edit the already-applied accounts
  migration.
- Preserve existing account IDs and balances. Set the name of any existing
  account to `Unnamed account` before enforcing the required name column.
- This backfill is valid even when multiple accounts receive the same label:
  names are not unique.
- On an empty accounts table, there is nothing to backfill.
- Do not retain a database default for new names. New account creation must
  explicitly supply a validated name, rather than silently receive the legacy
  label.
- Test migration from the preceding schema with both empty and populated
  accounts tables, verifying that existing IDs and balances remain unchanged.
- Migration execution remains an explicit user-controlled setup step.
