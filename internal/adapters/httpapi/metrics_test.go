package httpapi_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestMetricsEndpointAndHTTPObservations(t *testing.T) {
	m := observability.NewMetrics()
	h := httpapi.New(&service{}, slog.New(slog.NewTextHandler(io.Discard, nil))).WithMetrics(m)
	send := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	if w := send("GET", "/healthz"); w.Code != 200 {
		t.Fatalf("health: %d", w.Code)
	}
	if w := send("UNKNOWNSECRET", "/secret"); w.Code != 404 {
		t.Fatalf("unknown: %d", w.Code)
	}
	first := send("GET", "/metrics")
	if first.Code != 200 || !strings.HasPrefix(first.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("metrics response: %d %q", first.Code, first.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		`ledgersito_http_requests_total{method="GET",route="/healthz",status_class="2xx"} 1`,
		`ledgersito_http_requests_total{method="other",route="unmatched",status_class="4xx"} 1`,
	} {
		if !strings.Contains(first.Body.String(), want) {
			t.Errorf("missing %q in %s", want, first.Body.String())
		}
	}
	if strings.Contains(first.Body.String(), "UNKNOWNSECRET") || strings.Contains(first.Body.String(), "/secret") {
		t.Fatal("untrusted request data leaked into metrics")
	}
	if w := send("POST", "/metrics"); w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatalf("metrics method: %d, Allow=%q", w.Code, w.Header().Get("Allow"))
	}
	if w := send("GET", "/metrics?"); w.Code != 400 {
		t.Fatalf("metrics query: %d", w.Code)
	}
	second := send("GET", "/metrics")
	if first.Body.String() != second.Body.String() {
		t.Fatal("scrapes or rejected scrapes changed metrics")
	}
}
