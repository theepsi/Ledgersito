//go:build integration && initializationcheck

package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestStorageMetricsReplayAndRejectionIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	m := observability.NewMetrics()
	store := postgres.NewInstrumentedAPIStore(conn.Config(), m)
	businessErr := errors.New("business rejection")
	if _, err := store.CreateOnce(ctx, "rejected", "Name", func(createaccount.Store) (accounts.Response, error) {
		return accounts.Response{}, businessErr
	}); !errors.Is(err, businessErr) {
		t.Fatalf("got %v, want business rejection", err)
	}
	create := func(createaccount.Store) (accounts.Response, error) {
		return accounts.Response{Status: 201, Body: []byte(`{"created":true}`)}, nil
	}
	if _, err := store.CreateOnce(ctx, "replay", "Name", create); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOnce(ctx, "replay", "Name", func(createaccount.Store) (accounts.Response, error) {
		t.Fatal("replay invoked callback")
		return accounts.Response{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	text := string(m.Render())
	for _, want := range []string{
		`ledgersito_storage_operations_total{operation="create_account",outcome="error"} 1`,
		`ledgersito_storage_operations_total{operation="create_account",outcome="success"} 2`,
	} {
		if !strings.Contains(text, want+"\n") {
			t.Errorf("missing %s", want)
		}
	}
}
