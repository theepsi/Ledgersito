package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestTracingHTTPBehavior(t *testing.T) {
	for _, tt := range []struct {
		name, path, wantName string
		status, spanCount    int
		enabled              bool
	}{
		{name: "disabled", path: "/readyz", status: 200},
		{name: "health", path: "/healthz", wantName: "http /healthz", status: 200, spanCount: 1, enabled: true},
		{name: "readiness", path: "/readyz", wantName: "http /readyz", status: 200, spanCount: 2, enabled: true},
		{name: "metrics", path: "/metrics", wantName: "http unmatched", status: 200, spanCount: 1, enabled: true},
		{name: "unknown", path: "/private-account?key=private-key", wantName: "http unmatched", status: 404, spanCount: 1, enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			checker := readinessCheckerFunc(func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("readiness deadline lost")
				}
				end := observability.StartStorageSpan(ctx, "readiness")
				end(nil)
				return nil
			})
			h := httpapi.NewRouter(&service{}, nil, nil, nil, checker, logger).WithMetrics(observability.NewMetrics())
			if tt.enabled {
				h.WithTracer(observability.NewTracer(logger, rand.Reader))
			}
			request := httptest.NewRequest("GET", tt.path, nil)
			request.Header.Set("traceparent", "private-external-trace")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, request)
			if w.Code != tt.status || w.Header().Get("traceparent") != "" {
				t.Fatalf("unexpected response: %d %#v", w.Code, w.Header())
			}
			var spans []map[string]any
			for _, event := range decodeLogRecords(t, output.String()) {
				if event["event"] == "span_completed" {
					spans = append(spans, event)
				}
			}
			if len(spans) != tt.spanCount {
				t.Fatalf("got %d spans, want %d: %s", len(spans), tt.spanCount, output.String())
			}
			if tt.spanCount > 0 && spans[len(spans)-1]["span_name"] != tt.wantName {
				t.Fatalf("unexpected root: %#v", spans[len(spans)-1])
			}
			if tt.spanCount == 2 && (spans[0]["trace_id"] != spans[1]["trace_id"] || spans[0]["parent_span_id"] != spans[1]["span_id"]) {
				t.Fatalf("invalid parentage: %#v", spans)
			}
			for _, event := range spans {
				for _, field := range []string{"private-account", "private-key", "private-external-trace"} {
					if strings.Contains(toJSON(t, event), field) {
						t.Fatalf("span leaked %q: %#v", field, event)
					}
				}
			}
		})
	}
}

func TestTracingIDFailureDoesNotChangeResponse(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	h := httpapi.New(&service{}, logger).WithTracer(observability.NewTracer(logger, errorReader{}))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/healthz", nil))
	if response.Code != 200 || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected response %d %q", response.Code, response.Body.String())
	}
	if !strings.Contains(output.String(), "trace ID generation failed") || strings.Contains(output.String(), "span_completed") {
		t.Fatalf("unexpected log: %s", output.String())
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("random source failed")
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
