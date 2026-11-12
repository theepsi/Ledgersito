package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestStorageMetricsOnCancelledConnection(t *testing.T) {
	cfg, err := pgx.ParseConfig("postgres://localhost/ledger_test")
	if err != nil {
		t.Fatal(err)
	}

	m := observability.NewMetrics()
	store := postgres.NewInstrumentedAPIStore(cfg, m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CheckReadiness(ctx); err == nil {
		t.Fatal("expected cancelled readiness")
	}
	if _, err := store.List(ctx); err == nil {
		t.Fatal("expected cancelled list")
	}
	if _, err := store.ListTransactions(ctx); err == nil {
		t.Fatal("expected cancelled history")
	}
	if _, err := store.Summarize(ctx, nil); err == nil {
		t.Fatal("expected cancelled analytics")
	}
	if _, err := store.CreateOnce(ctx, "", "", func(createaccount.Store) (accounts.Response, error) {
		t.Fatal("unexpected callback")
		return accounts.Response{}, nil
	}); err == nil {
		t.Fatal("expected cancelled creation")
	}
	if _, err := store.TransferOnce(ctx, "", transfer.Request{}, func(transfers.Store) (transfers.Response, error) {
		t.Fatal("unexpected callback")
		return transfers.Response{}, nil
	}); err == nil {
		t.Fatal("expected cancelled transfer")
	}
	text := string(m.Render())
	for _, operation := range []string{"readiness", "list_accounts", "list_transactions", "analytics", "create_account", "transfer"} {
		want := `ledgersito_storage_operations_total{operation="` + operation + `",outcome="error"} 1`
		if !strings.Contains(text, want+"\n") {
			t.Errorf("missing %s", want)
		}
	}

}

func TestReadinessRecordsHTTPAndStorageMetrics(t *testing.T) {
	cfg, err := pgx.ParseConfig("postgres://localhost/ledger_test")
	if err != nil {
		t.Fatal(err)
	}
	m := observability.NewMetrics()
	store := postgres.NewInstrumentedAPIStore(cfg, m)
	handler := httpapi.NewRouter(nil, nil, nil, nil, store, slog.New(slog.NewTextHandler(io.Discard, nil))).WithMetrics(m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("readiness returned %d", response.Code)
	}
	text := string(m.Render())
	for _, want := range []string{
		`ledgersito_http_requests_total{method="GET",route="/readyz",status_class="5xx"} 1`,
		`ledgersito_storage_operations_total{operation="readiness",outcome="error"} 1`,
	} {
		if !strings.Contains(text, want+"\n") {
			t.Errorf("missing %s", want)
		}
	}
}
