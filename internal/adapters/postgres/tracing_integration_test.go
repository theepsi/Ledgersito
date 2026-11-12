//go:build integration && initializationcheck

package postgres_test

import (
	"bytes"
	"crypto/rand"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestReadinessTraceIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	if _, err := conn.Exec(ctx, `
CREATE TABLE goose_db_version (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL
);
INSERT INTO goose_db_version (version_id, is_applied) VALUES (5, true)`); err != nil {
		t.Fatal(err)
	}
	before := ledgerContents(t, ctx, conn)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	store := postgres.NewAPIStore(conn.Config())
	handler := httpapi.NewRouter(nil, nil, nil, nil, store, logger).
		WithTracer(observability.NewTracer(logger, rand.Reader))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx))
	if w.Code != 200 || w.Body.String() != `{"status":"ready"}` {
		t.Fatalf("unexpected response %d %q", w.Code, w.Body.String())
	}
	spans := decodeTraceEvents(t, output.String())
	if len(spans) != 2 || spans[0]["span_name"] != "storage readiness" ||
		spans[0]["outcome"] != "ok" || spans[1]["span_name"] != "http /readyz" ||
		spans[1]["outcome"] != "ok" || spans[0]["parent_span_id"] != spans[1]["span_id"] ||
		spans[0]["trace_id"] != spans[1]["trace_id"] {
		t.Fatalf("unexpected readiness spans: %#v", spans)
	}
	if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
		t.Fatal("readiness tracing changed ledger data")
	}
}
