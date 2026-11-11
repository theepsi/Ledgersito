# Ledger Analytics

## Status and purpose

Approved and implemented.

Expose read-only ledger-wide statistics to learn the difference between
transfer activity (a flow) and funds currently held in circulation (a stock).
The statistics complement transaction history and reconciliation; they do
not replace either.

## Version 1 scope

- Provide exact `GET /analytics`.
- Require a successfully initialized ledger. An uninitialized ledger returns
  `409 ledger_not_initialized`.
- With no time bounds, report lifetime totals from initialization through one
  consistent database snapshot. Optionally accept a bounded time window as
  described under HTTP behavior.
- Include only successfully posted transfers. Exclude the `system-opening`
  transaction and do not count idempotent retries or replay records.
- Keep the endpoint read-only. Do not repair, initialize, reconcile, or change
  ledger state.
- Assume the existing ledger invariants hold. Reconciliation remains the
  authoritative diagnostic for corrupted or inconsistent persisted state.

## Posting timestamps

- Migration 5 adds nullable `posted_at TIMESTAMPTZ` to
  `ledger_transactions`. PostgreSQL records `clock_timestamp()` when a
  transaction transitions from unposted to posted. The timestamp is the
  database's wall-clock observation of posting, not an exact commit time or a
  strict commit-order sequence.
- Transactions already posted when the migration runs keep `posted_at = NULL`;
  their actual posting times are unknown and must not be fabricated.
- The existing `GET /transactions` response does not expose `posted_at` in
  this change. Its deterministic entry-ID ordering remains unchanged.

## Metric definitions

- **Transfer count:** the number of posted transfer transactions after the
  opening transaction. In the current model, every posted non-opening
  transaction is a transfer; if another transaction kind is introduced, its
  classification must be specified rather than implicitly counted here.
- **Total transfer volume:** the sum of each transfer's amount counted once,
  using its negative source entry. This includes both reservoir distributions
  and ordinary-to-ordinary transfers.
- **Reservoir distribution:** count and total amount of transfers sourced from
  the reservoir. This is cumulative flow out of the reservoir, not additional
  supply.
- **Ordinary-to-ordinary transfer volume:** count and total amount of transfers
  between ordinary accounts. It represents subsequent movement of already
  distributed funds and may exceed the original supply over time.
- **Funds in circulation:** the current sum of ordinary-account balances in
  the snapshot. It is a stock, not a sum of historical transfers.
- **Reservoir balance and total supply:** report the current reservoir balance
  and initialized fixed supply so the remaining undistributed funds and
  conservation relationship are visible.
- **Timestamp coverage:** report the count and total volume of posted transfers
  whose `posted_at` is unknown. This is a whole-ledger value, including in
  bounded-window responses, so callers can see how much legacy activity cannot
  be assigned to a period.

Under current ledger rules, cumulative reservoir distribution equals the
current sum of ordinary balances: the reservoir cannot receive transfers, and
ordinary balances cannot go negative. Report these as distinct flow and stock
metrics for learning, but do not add them together as if they were separate
portions of supply. Total transfer volume is the sum of reservoir distributions
and ordinary-to-ordinary transfer volume; it can exceed both the supply and
the current funds in circulation.

## HTTP behavior

- Return `200 OK` with a JSON object containing `period`, transfer-flow metrics,
  timestamp coverage, and current stock metrics.
- `period` is the string `"lifetime"` for an unbounded request, or an object
  with normalized `from` and `to` RFC 3339 strings for a bounded request.
- Accept either no time-bound query parameters or both `from` and `to`. Bounds
  must be RFC 3339 timestamps with an explicit UTC offset and must satisfy
  `from < to`. Interpret the window as half-open:
  `from <= posted_at < to`; normalize returned bounds to UTC.
- Reject a single bound, malformed or reversed bounds, duplicate bounds, and
  unsupported query parameters with `400 invalid_request`.
- With no bounds, `period` is `"lifetime"` and transfer-flow metrics include
  all posted transfers, including pre-migration rows with unknown timestamps.
  With bounds, `period` reports the normalized bounds and flow metrics include
  only transfers whose `posted_at` falls in the window. Unknown timestamps are
  excluded from the windowed flow metrics and reported separately under
  timestamp coverage.
- Current funds in circulation, reservoir balance, and total supply are values
  from the response's current database snapshot, not historical balances as of
  the window's end.
- Use the existing JSON error envelope and five-second request deadline.
- Other methods return `405` with `Allow: GET`; unknown paths use the existing
  JSON `404` behavior.
- Return `503 request_timeout` on deadline expiration and `500 internal_error`
  on storage or encoding failures. Never turn a failure into zero-valued
  successful statistics.
- Read all metrics from one PostgreSQL `REPEATABLE READ`, read-only snapshot so
  concurrent transfers cannot produce a mixed report.

Illustrative lifetime response for a 250-sitito reservoir distribution followed
by a 70-sitito ordinary-to-ordinary transfer, with no legacy untimestamped
transfers:

```json
{
  "period": "lifetime",
  "transfers": {
    "count": 2,
    "total_volume": {"sititos": "320", "currency": "SITO"},
    "reservoir_distribution": {
      "count": 1,
      "volume": {"sititos": "250", "currency": "SITO"}
    },
    "ordinary_to_ordinary": {
      "count": 1,
      "volume": {"sititos": "70", "currency": "SITO"}
    }
  },
  "timestamp_coverage": {
    "untimestamped_transfer_count": 0,
    "untimestamped_transfer_volume": {"sititos": "0", "currency": "SITO"}
  },
  "current": {
    "funds_in_circulation": {"sititos": 250, "currency": "SITO"},
    "reservoir_balance": {"sititos": 99999750, "currency": "SITO"},
    "total_supply": {"sititos": 100000000, "currency": "SITO"}
  }
}
```

For a bounded request, the period is represented as:

```json
{
  "period": {
    "from": "2026-01-01T00:00:00Z",
    "to": "2026-02-01T00:00:00Z"
  }
}
```

The decimal strings for cumulative volume totals preserve exact values:
transfer volume can exceed int64 even though the fixed supply and each account
balance fit in int64. Snapshot balances and supply remain int64. Use exact
PostgreSQL numeric aggregation and do not narrow aggregate results before
encoding.

## Acceptance scenarios

- An uninitialized ledger returns `409 ledger_not_initialized` and is
  unchanged.
- An initialized ledger with no transfers reports zero counts and volumes,
  zero funds in circulation, the full supply in the reservoir, and the fixed
  total supply.
- After a reservoir distribution and an ordinary-to-ordinary transfer, counts
  and volumes match the posted transactions; the response distinguishes
  cumulative transfer activity from current ordinary balances.
- Repeated ordinary-to-ordinary transfers increase transfer volume but do not
  increase the measured funds in circulation.
- The opening transaction and same-key transfer replays are not counted as
  transfers.
- `from` and `to` bounds include the start and exclude the end. Lifetime
  statistics include legacy transfers with unknown posting timestamps;
  bounded-window statistics exclude them and report their count and volume
  separately.
- Current stock metrics are computed from the same snapshot as the flow
  metrics, but are not presented as historical stocks for a requested window.
- Concurrent transfers do not cause statistics assembled from different
  database snapshots.
- Aggregate volumes larger than int64 remain exact in the response.
- A storage, timeout, or encoding failure returns an error rather than
  plausible zero statistics.
- Listing analytics does not modify account balances, entries, transactions,
  initialization metadata, or replay records.

Use focused application and HTTP tests, plus opt-in PostgreSQL integration
tests for persisted aggregates and concurrent-snapshot behavior. Database
preparation and migration execution remain user-controlled.

## Non-goals

- Per-account rankings, account-specific filtering, pagination, dashboards, or
  event delivery.
- Daily calendar buckets, timezone-specific reporting, or strict commit-order
  guarantees.
- Reconciliation findings, ledger repair, or changes to transfer semantics.
- New issuance, changes to fixed supply, or support for real money.

## Implementation notes

- Cumulative flow volumes and untimestamped legacy volume are encoded as
  decimal strings to preserve exact PostgreSQL `numeric` aggregates beyond
  int64 and JavaScript's exact integer range.
- Flow and current-stock metrics are read by one aggregate query inside a
  PostgreSQL `REPEATABLE READ`, read-only transaction.
- Current stock values in bounded-window responses remain current snapshot
  values; they are not historical balances at the window end.
- `GET /transactions` continues to omit `posted_at`; this endpoint uses it
  internally to filter bounded flow metrics.
