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
  overflow errors. Behavior agreed; tests and implementation are not yet added.
