package postgres_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestReadinessTraceIncludesStorageFailure(t *testing.T) {
	cfg, err := pgx.ParseConfig("postgres://localhost/ledger_test")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	store := postgres.NewAPIStore(cfg)
	handler := httpapi.NewRouter(nil, nil, nil, nil, store, logger).
		WithTracer(observability.NewTracer(logger, rand.Reader))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatalf("got status %d", w.Code)
	}
	spans := decodeTraceEvents(t, output.String())
	if len(spans) != 2 || spans[0]["span_name"] != "storage readiness" ||
		spans[0]["outcome"] != "error" || spans[1]["span_name"] != "http /readyz" ||
		spans[1]["outcome"] != "error" || spans[0]["parent_span_id"] != spans[1]["span_id"] ||
		spans[0]["trace_id"] != spans[1]["trace_id"] {
		t.Fatalf("unexpected readiness spans: %#v", spans)
	}
}

func decodeTraceEvents(t *testing.T, output string) []map[string]any {
	t.Helper()
	var spans []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["event"] == "span_completed" {
			spans = append(spans, event)
		}
	}
	return spans
}
