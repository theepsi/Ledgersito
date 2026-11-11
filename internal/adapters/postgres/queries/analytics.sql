-- name: AnalyticsSnapshot :one
WITH transfer_sources AS (
    SELECT t.id, t.posted_at, bool_or(a.role = 'reservoir') AS from_reservoir,
        -sum(e.amount::numeric) AS volume
    FROM ledger_transactions t
    JOIN entries e ON e.transaction_id = t.id
    JOIN accounts a ON a.id = e.account_id
    WHERE t.posted AND t.id <> 'system-opening' AND e.amount < 0
    GROUP BY t.id, t.posted_at
), selected_transfers AS (
    SELECT id, posted_at, from_reservoir, volume
    FROM transfer_sources
    WHERE NOT sqlc.arg(windowed)::boolean
        OR (
            posted_at >= sqlc.narg(window_from)::timestamptz
            AND posted_at < sqlc.narg(window_to)::timestamptz
        )
), transfer_metrics AS (
    SELECT
        count(*) AS transfer_count,
        coalesce(sum(volume), 0)::text AS total_transfer_volume,
        count(*) FILTER (WHERE from_reservoir) AS reservoir_distribution_count,
        coalesce(sum(volume) FILTER (WHERE from_reservoir), 0)::text AS reservoir_distribution_volume,
        count(*) FILTER (WHERE NOT from_reservoir) AS ordinary_transfer_count,
        coalesce(sum(volume) FILTER (WHERE NOT from_reservoir), 0)::text AS ordinary_transfer_volume
    FROM selected_transfers
), timestamp_coverage AS (
    SELECT count(*) AS untimestamped_transfer_count,
        coalesce(sum(volume), 0)::text AS untimestamped_transfer_volume
    FROM transfer_sources
    WHERE posted_at IS NULL
)
SELECT
    EXISTS (SELECT 1 FROM ledger_initialization) AS initialized,
    transfer_metrics.transfer_count,
    transfer_metrics.total_transfer_volume,
    transfer_metrics.reservoir_distribution_count,
    transfer_metrics.reservoir_distribution_volume,
    transfer_metrics.ordinary_transfer_count,
    transfer_metrics.ordinary_transfer_volume,
    timestamp_coverage.untimestamped_transfer_count,
    timestamp_coverage.untimestamped_transfer_volume,
    coalesce(
        (SELECT sum(balance) FROM accounts WHERE role = 'ordinary'),
        0
    )::bigint AS funds_in_circulation,
    coalesce(
        (SELECT balance FROM accounts WHERE role = 'reservoir' LIMIT 1),
        0
    )::bigint AS reservoir_balance,
    coalesce(
        (SELECT supply FROM ledger_initialization LIMIT 1),
        0
    )::bigint AS total_supply
FROM transfer_metrics
CROSS JOIN timestamp_coverage;
