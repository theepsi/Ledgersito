# Money

## Purpose and scope

Represent exact amounts in the project's single fictional currency and perform
safe addition and subtraction. Learn minor-unit arithmetic, value semantics,
and explicit error handling without HTTP or database dependencies.

## Currency and representation

- The currency is Sitos, identified by the project-specific code `SITO`.
  This is not an ISO currency code.
- One Sito equals 100 sititos. A sitito is the smallest supported unit.
- Represent amounts as signed `int64` counts of sititos, never floating-point
  values. For example, 12.50 Sitos is represented as `1250`.
- All `int64` amounts are valid, including zero, negative values, and both
  limits: `-9223372036854775808` and `9223372036854775807` sititos.
- Construction accepts an amount in sititos; callers do not choose a currency.
  Money always represents SITO.

## Operations and invariants

- Construction preserves the exact supplied amount. Callers can read that
  amount in sititos without decimal conversion.
- Addition returns the mathematical sum when it fits in `int64`.
- Subtraction returns the mathematical difference when it fits in `int64`.
  Subtracting a negative amount increases the result.
- Operations return new values and leave both operands unchanged, on success
  and failure.
- No operation silently wraps, clamps, rounds, or changes the currency.

## Overflow behavior

Addition and subtraction return `(Money, error)`.

- A representable result returns the exact Money value and a nil error.
- A result outside the `int64` range returns a non-nil overflow error rather
  than panicking. Results below the minimum follow the same error rule.
- On error, the returned Money value is not a valid result of the operation
  and must be ignored. Its amount is not part of this contract.
- Negative results are not errors when they fit in the supported range.

## Acceptance examples

All amounts below are in sititos. `MIN` means `-9223372036854775808`;
`MAX` means `9223372036854775807`.

| Operation | Inputs | Expected outcome |
|---|---|---|
| Construct and read | `0`, `1250`, `-325`, `MIN`, `MAX` | Preserve each supplied amount exactly |
| Add | `1250`, `275` | `1525`, no error |
| Add | `250`, `-250` | `0`, no error |
| Add | `-250`, `-75` | `-325`, no error |
| Add | `MAX`, `0` | `MAX`, no error |
| Add | `MIN`, `0` | `MIN`, no error |
| Add | `MAX`, `MIN` | `-1`, no error |
| Add | `MAX`, `1` | Overflow error |
| Add | `MIN`, `-1` | Overflow error |
| Subtract | `1250`, `1500` | `-250`, no error |
| Subtract | `1250`, `-275` | `1525`, no error |
| Subtract | `250`, `250` | `0`, no error |
| Subtract | `MAX`, `0` | `MAX`, no error |
| Subtract | `MIN`, `0` | `MIN`, no error |
| Subtract | `MIN`, `MIN` | `0`, no error |
| Subtract | `-1`, `MIN` | `MAX`, no error |
| Subtract | `MAX`, `-1` | Overflow error |
| Subtract | `MIN`, `1` | Overflow error |
| Subtract | `0`, `MIN` | Overflow error |

Tests must also verify that operands remain unchanged after successful and
overflowing operations. These are domain unit tests and need neither Docker
nor PostgreSQL.

## Non-goals

- Other currencies, currency selection, or exchange rates.
- Decimal/string parsing, display formatting, JSON, or database serialization.
- Multiplication, division, rounding, or a separate negation operation.
- Account balance restrictions or the rule that transfer amounts must be
  positive; those belong to later specs.
- Persistence, HTTP endpoints, and ledger transactions.
