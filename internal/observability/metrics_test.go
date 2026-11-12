package observability_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestMetricsExposition(t *testing.T) {
	m := observability.NewMetrics()
	m.ObserveHTTP("GET", "/healthz", 200, 8*time.Millisecond)
	m.ObserveHTTP("private-method", "/private-account", 404, 2*time.Second)
	m.ObserveStorage("transfer", errors.New("private database error"), 25*time.Millisecond)
	m.ObserveStorage("transfer", nil, time.Millisecond)

	text := string(m.Render())
	for _, line := range []string{
		`ledgersito_http_requests_total{method="GET",route="/healthz",status_class="2xx"} 1`,
		`ledgersito_http_request_duration_seconds_bucket{method="GET",route="/healthz",status_class="2xx",le="0.01"} 1`,
		`ledgersito_http_request_duration_seconds_bucket{method="GET",route="/healthz",status_class="2xx",le="0.005"} 0`,
		`ledgersito_http_request_duration_seconds_bucket{method="GET",route="/healthz",status_class="2xx",le="+Inf"} 1`,
		`ledgersito_http_request_duration_seconds_count{method="GET",route="/healthz",status_class="2xx"} 1`,
		`ledgersito_http_requests_total{method="other",route="unmatched",status_class="4xx"} 1`,
		`ledgersito_storage_operations_total{operation="transfer",outcome="error"} 1`,
		`ledgersito_storage_operations_total{operation="transfer",outcome="success"} 1`,
		`ledgersito_storage_operation_duration_seconds_count{operation="transfer",outcome="error"} 1`,
		`ledgersito_storage_operation_duration_seconds_sum{operation="transfer",outcome="success"} 0.001`,
	} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("missing %q in\n%s", line, text)
		}
	}
	for _, private := range []string{"private-method", "private-account", "private database error"} {
		if strings.Contains(text, private) {
			t.Errorf("metrics leaked %q", private)
		}
	}
}

func TestMetricsConcurrentRecordingAndScraping(t *testing.T) {
	m := observability.NewMetrics()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				m.ObserveHTTP("GET", "/healthz", 200, time.Millisecond)
				_ = m.Render()
			}
		}()
	}
	wg.Wait()
	want := `ledgersito_http_requests_total{method="GET",route="/healthz",status_class="2xx"} 1000`
	if !strings.Contains(string(m.Render()), want+"\n") {
		t.Fatal("concurrent observations were lost")
	}
}
