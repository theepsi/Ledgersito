package observability

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

var bounds = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type histogram struct {
	count   uint64
	sum     float64
	buckets [len(bounds)]uint64
}

func (h *histogram) observe(seconds float64) {
	h.count++
	h.sum += seconds
	for i, bound := range bounds {
		if seconds <= bound {
			h.buckets[i]++
		}
	}
}

type httpKey struct {
	method, route, status string
}

type storageKey struct {
	operation, outcome string
}

type Metrics struct {
	mu      sync.Mutex
	http    map[httpKey]histogram
	storage map[storageKey]histogram
}

func NewMetrics() *Metrics {
	return &Metrics{http: make(map[httpKey]histogram), storage: make(map[storageKey]histogram)}
}

func (m *Metrics) ObserveHTTP(method, route string, status int, duration time.Duration) {
	switch method {
	case "GET", "POST", "HEAD":
	default:
		method = "other"
	}
	switch route {
	case "/accounts", "/transfers", "/transactions", "/analytics", "/healthz", "/readyz":
	default:
		route = "unmatched"
	}
	class := "other"
	if status >= 200 && status < 600 {
		class = strconv.Itoa(status/100) + "xx"
	}
	key := httpKey{method, route, class}
	m.mu.Lock()
	h := m.http[key]
	h.observe(duration.Seconds())
	m.http[key] = h
	m.mu.Unlock()
}

func (m *Metrics) ObserveStorage(operation string, err error, duration time.Duration) {
	switch operation {
	case "create_account", "list_accounts", "transfer", "list_transactions", "analytics", "readiness":
	default:
		operation = "other"
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	key := storageKey{operation, outcome}
	m.mu.Lock()
	h := m.storage[key]
	h.observe(duration.Seconds())
	m.storage[key] = h
	m.mu.Unlock()
}

type series struct {
	labels string
	data   histogram
}

func (m *Metrics) Render() []byte {
	m.mu.Lock()
	httpSeries := make([]series, 0, len(m.http))
	for key, data := range m.http {
		httpSeries = append(httpSeries, series{
			labels: fmt.Sprintf(`method="%s",route="%s",status_class="%s"`, key.method, key.route, key.status),
			data:   data,
		})
	}
	storageSeries := make([]series, 0, len(m.storage))
	for key, data := range m.storage {
		storageSeries = append(storageSeries, series{
			labels: fmt.Sprintf(`operation="%s",outcome="%s"`, key.operation, key.outcome),
			data:   data,
		})
	}
	m.mu.Unlock()
	sort.Slice(httpSeries, func(i, j int) bool { return httpSeries[i].labels < httpSeries[j].labels })
	sort.Slice(storageSeries, func(i, j int) bool { return storageSeries[i].labels < storageSeries[j].labels })

	var out bytes.Buffer
	writeFamily(&out, "ledgersito_http_requests_total", "ledgersito_http_request_duration_seconds", "HTTP requests", httpSeries)
	writeFamily(&out, "ledgersito_storage_operations_total", "ledgersito_storage_operation_duration_seconds", "Storage operations", storageSeries)
	return out.Bytes()
}

func writeFamily(out *bytes.Buffer, counter, duration, description string, series []series) {
	fmt.Fprintf(out, "# HELP %s %s completed.\n# TYPE %s counter\n", counter, description, counter)
	for _, s := range series {
		fmt.Fprintf(out, "%s{%s} %d\n", counter, s.labels, s.data.count)
	}
	fmt.Fprintf(out, "# HELP %s %s duration in seconds.\n# TYPE %s histogram\n", duration, description, duration)
	for _, s := range series {
		for i, bound := range bounds {
			fmt.Fprintf(out, "%s_bucket{%s,le=%q} %d\n", duration, s.labels, strconv.FormatFloat(bound, 'f', -1, 64), s.data.buckets[i])
		}
		fmt.Fprintf(out, "%s_bucket{%s,le=\"+Inf\"} %d\n", duration, s.labels, s.data.count)
		fmt.Fprintf(out, "%s_sum{%s} %s\n", duration, s.labels, strconv.FormatFloat(s.data.sum, 'g', -1, 64))
		fmt.Fprintf(out, "%s_count{%s} %d\n", duration, s.labels, s.data.count)
	}
}
