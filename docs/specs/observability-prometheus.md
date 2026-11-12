# Observability: Local Prometheus Collection

## Status

Draft for discussion. Not approved for implementation or service setup.

The proposed topology is a native, manually started local Prometheus process.
This is a recommendation, not an agreed installation choice. Confirm or revise
it with the user before implementation. Approval of this spec would not itself
authorize installations, downloads, service startup, or database preparation.

## Purpose and scope

Learn the difference between exposing cumulative metrics and collecting a
history that can be queried over time. Prometheus periodically pulls the
existing `GET /metrics` endpoint, stores timestamped samples locally, and
provides PromQL queries and graphs in its built-in browser UI.

Use the existing [HTTP and storage metrics](observability-metrics.md) unchanged.
No Go client library, exporter, collector intermediary, or new application
instrumentation is needed. This slice adds collection configuration and public
instructions, not changes to ledger behavior.

Operational measurements do not replace ledger analytics or reconciliation.
Counters describe attempts, including retries and idempotent replays, rather
than distinct committed transactions.

## Proposed local topology

- The API continues running on its existing default `127.0.0.1:8080`.
- One native Prometheus process on the same host scrapes that address.
- Prometheus listens explicitly on `127.0.0.1:9090`, not all interfaces.
  Its built-in targets and query pages are the only UI in this slice.
- Startup and shutdown are manual. Do not install an operating-system service,
  add autostart, change firewall rules, or start other Compose services.
- Prometheus does not receive database credentials or access PostgreSQL.
- An API address override requires an explicit matching scrape target change;
  no automatic discovery or API binding change is introduced.

A bridged Docker container's `127.0.0.1` is not the host's loopback interface.
Do not solve that mismatch by exposing the unauthenticated API on all
interfaces. If the user prefers Docker, revise the topology before approval:
Linux host networking is an option, but is platform-specific and would need an
explicitly opt-in service. The existing PostgreSQL Compose workflow stays
unchanged in this proposal.

## Collection contract

Provide a small tracked example configuration during implementation, with:

- One static job named `ledgersito`.
- One target, `127.0.0.1:8080`, using HTTP and `/metrics`, with no query string.
- A scrape interval of 15 seconds and timeout of 5 seconds.
- No remote write, federation, service discovery, additional exporters,
  recording rules, alert rules, or Prometheus self-scraping.
- The default target labels `job` and `instance`, without caller or ledger
  values in additional labels.

Prometheus collects the existing counters and classic duration histograms,
including bucket, sum, and count series. Preserve names, units, labels, and
buckets from the approved metrics spec. Do not introduce native histograms or
convert storage-operation latency into a claim about individual SQL queries.

Prometheus also supplies scrape diagnostics such as
`up{job="ledgersito"}`. A value of `1` means the metrics scrape succeeded;
`0` means an attempted scrape failed. A missing series can mean there is no
configured or collected target. None of these proves `/readyz` succeeds or
that ledger invariants hold. `/metrics` does not access PostgreSQL, so it can
remain scrapeable while readiness fails.

Scraping does not generate readiness or application traffic. The exact
`/metrics` path remains excluded from the application's HTTP metrics, but
existing request logs still record scrapes. When local tracing is enabled,
scrapes also emit existing HTTP span records. Do not suppress or otherwise
change those behaviors in this slice.

## Local history and resource behavior

- Propose a 24-hour time retention and a 1 GB TSDB size retention, both set
  explicitly in documented startup arguments. Either retention limit may
  shorten available history.
- Document that size retention is not a hard cap on total disk consumption:
  head/WAL data and retention cleanup timing require additional disk space.
- Use an explicitly selected local data directory outside tracked source
  files. Do not commit samples, logs, downloaded binaries, or machine-specific
  paths; do not alter ignore rules for private guides.
- Restarting Prometheus with the same data directory retains collected history
  within retention limits. Deleting its data is a separate, user-controlled
  action, never a setup or troubleshooting prerequisite.
- Restarting the API resets its process-local counters. Prometheus retains
  earlier samples; `rate` and `increase` account for observed counter resets.
  Work between scrapes, including resets, cannot always be reconstructed.
- Stopping Prometheus creates a collection gap. Restarting it does not backfill
  missed samples. Stored samples and derived rates are operational estimates,
  not exact durable accounting records.

## Learning queries

The public instructions should explain these examples and their units.

Scrape health:

```promql
up{job="ledgersito"}
```

Completed application HTTP requests per second, excluding health/readiness
traffic to avoid confusing manual probe traffic with application activity:

```promql
sum(rate(ledgersito_http_requests_total{
  job="ledgersito",route!~"/healthz|/readyz"
}[5m]))
```

HTTP request rates by status class:

```promql
sum by (status_class) (
  rate(ledgersito_http_requests_total{job="ledgersito"}[5m])
)
```

Estimated 95th-percentile HTTP latency in seconds, grouped by route:

```promql
histogram_quantile(
  0.95,
  sum by (le, route) (
    rate(ledgersito_http_request_duration_seconds_bucket{
      job="ledgersito"
    }[5m])
  )
)
```

Storage attempts per second by operation and outcome:

```promql
sum by (operation, outcome) (
  rate(ledgersito_storage_operations_total{job="ledgersito"}[5m])
)
```

Explain how status-class rates can be used to calculate a 5xx fraction, and
storage outcome rates an operation-error fraction, using matching populations
and windows. Storage errors include business rejections, not just database
failures.

Only observed application label combinations exist. An empty result is not
automatically zero, and no traffic can produce undefined ratios or quantiles.
Do not present absent data, an unavailable target, or division by zero as a
healthy zero-error result. Rate calculations need at least two samples, and
short bursts or idle learning workloads can make five-minute graphs sparse.
Histogram quantiles are bucket-based estimates, especially noisy with few
observations.

## Acceptance scenarios and validation

Implementation should provide a tracked example config and public walkthrough,
linked from the existing setup documentation. The walkthrough must distinguish
commands for reviewing configuration from commands that start processes.

- Validate the example configuration with `promtool check config` from the
  user-approved Prometheus distribution before claiming it is runnable.
- With user-authorized processes running, confirm Prometheus's listener is
  loopback-only, and its target page reports the configured job and address.
- After successful scrapes, `up{job="ledgersito"}` is `1`, and API metric
  samples appear after corresponding requests have been observed.
- Use read-only HTTP traffic to demonstrate collection and graphing, including
  a successful request and an intentionally unmatched path for a 4xx result.
  Do not create transfers or accounts merely to populate graphs.
- Repeated scrapes alone do not increment the application's HTTP counters.
  Existing logs, and spans when enabled, still reflect the scrape requests.
- Demonstrate a failed scrape and subsequent recovery only with separately
  authorized process control or a disposable unreachable target. Do not stop
  the user's existing API or database without permission. Confirm failure is
  visible rather than reported as zero application errors.
- If an API restart is authorized, confirm old samples remain and new
  counter values reset; demonstrate reset-aware queries without claiming
  exact reconstruction of unsampled traffic.
- If a Prometheus restart is authorized, reuse the selected data directory
  and confirm earlier samples remain within retention.
- Confirm setup requires no migrations, initialization, database writes,
  tracing changes, or modification of existing API instrumentation.

Manual acceptance checks remain explicitly unexecuted until the necessary
tools, processes, and permissions are available. Do not install dependencies
or start services as a side effect of validating this documentation.

## Decisions to confirm before approval

- Native local process versus an explicitly specified container alternative.
- Proposed 15-second interval, 5-second timeout, 24-hour retention, and 1 GB
  size retention.
- User-selected distribution/version, acquisition method, and local data
  directory. Do not use the AUR or infer installation permission.

## Non-goals

- Grafana, Alertmanager, alerts, dashboards, remote storage, cloud telemetry,
  public network access, production deployment, or authentication changes.
- Exporting logs or local spans to Prometheus, trace exemplars, distributed
  tracing, or replacing the existing local tracing format.
- PostgreSQL server metrics, operating-system metrics, per-account metrics,
  new Go dependencies, or changing the approved metric contract.
- Automatic traffic generators, write workloads, exact transaction counting,
  database preparation, migrations, initialization, or ledger repairs.

## References

- [Prometheus overview](https://prometheus.io/docs/introduction/overview/)
- [Prometheus getting started](https://prometheus.io/docs/prometheus/latest/getting_started/)
- [Prometheus storage](https://prometheus.io/docs/prometheus/latest/storage/)
