# Observability: Local Prometheus Collection

## Status

Approved for implementation with the Docker host-networking topology below.
The Compose configuration and public walkthrough are implemented. Static
configuration checks are separate from the pending Prometheus semantic and
live acceptance checks.

Approval does not authorize installations, image downloads, container
execution, service startup, or database preparation. Those remain explicit,
user-controlled actions.

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

## Local topology

- The API continues running on its existing default `127.0.0.1:8080`.
- One Prometheus container uses Linux host networking to scrape that address.
  Pin the official image to `prom/prometheus:v3.13.5`, the LTS patch selected
  for this slice, rather than a floating tag.
- Keep it opt-in through the standalone `compose.prometheus.yaml` file, with
  project name `ledgersito-observability`, separate from the existing
  PostgreSQL Compose project. Do not merge the two files for startup or cleanup.
- Prometheus listens explicitly on `127.0.0.1:9090`, not all interfaces.
  Its built-in targets and query pages are the only UI in this slice.
- Startup and shutdown are manual. Do not install an operating-system service,
  add autostart, change firewall rules, or start other Compose services.
- Prometheus does not receive database credentials or access PostgreSQL.
- An API address override requires an explicit matching scrape target change;
  no automatic discovery or API binding change is introduced.

A bridged Docker container's `127.0.0.1` is not the host's loopback interface.
Host networking shares the host's network namespace instead: software and
storage are container-managed, but network isolation is not provided. This
slice targets Linux Docker Engine, not a portable Docker Desktop setup.
Do not expose the unauthenticated API on all interfaces to work around
networking differences. The existing PostgreSQL Compose workflow is unchanged.

## Collection contract

Provide the tracked configuration at `observability/prometheus.yml`, mounted
read-only into the container, with:

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

- Use a 24-hour time retention and a 1 GB TSDB size retention, both set
  explicitly in the Compose command arguments. Either retention limit may
  shorten available history.
- Document that size retention is not a hard cap on total disk consumption:
  head/WAL data and retention cleanup timing require additional disk space.
- Store history at `/prometheus` in the dedicated Compose-managed
  `prometheus_data` named volume, outside the worktree. Do not commit samples,
  logs, downloaded binaries, or machine-specific paths; do not alter ignore
  rules for private guides.
- Restarting Prometheus with the same volume retains collected history
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

Provide a tracked config and public walkthrough in the existing setup
documentation. The walkthrough must distinguish configuration review from
image downloads, temporary validation containers, and service startup.

- Validate the Compose model with `docker compose config`, without starting
  services. Validate the scrape configuration with `promtool check config`
  from the pinned image once image download and a temporary validation
  container are authorized, before claiming semantic validation passed.
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
- Stopping or removing the observability project preserves its history by
  default. Explicit volume removal deletes only observability history;
  PostgreSQL's container and data volume are not part of this Compose model.
- Confirm setup requires no migrations, initialization, database writes,
  tracing changes, or modification of existing API instrumentation.

Manual acceptance checks remain explicitly unexecuted until the necessary
tools, processes, and permissions are available. Do not install dependencies
or start services as a side effect of validating this documentation.

## Setup choices and validation status

The agreed design uses Docker host networking, a 15-second scrape interval,
5-second timeout, 24-hour retention, and 1 GB size retention. No native
Prometheus installation or AUR package is needed.

Image acquisition, container execution, and live acceptance checks require
separate permission. Until then, `promtool` and live checks remain unexecuted;
static checks alone do not prove successful collection.

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
