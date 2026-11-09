//go:build integration && initializationcheck

package postgres_test

import (
	"reflect"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/history"
)

func TestTransactionHistoryIntegration(t *testing.T) {
	t.Run("empty ledger", func(t *testing.T) {
		conn, ctx := initializationDB(t, false)
		before := ledgerContents(t, ctx, conn)
		got, err := history.New(postgres.NewAPIStore(conn.Config())).List(ctx)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %#v, %v", got, err)
		}
		if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
			t.Fatal("listing empty history changed ledger state")
		}
	})

	conn, ctx, transfers := transferDB(t, true)
	store := postgres.NewAPIStore(conn.Config())
	before := ledgerContents(t, ctx, conn)
	got, err := history.New(store).List(ctx)
	if err != nil || len(got) != 1 || got[0].ID != "system-opening" ||
		len(got[0].Entries) != 2 || got[0].Entries[0].Amount.Sititos != -100000000 ||
		got[0].Entries[1].Amount.Sititos != 100000000 {
		t.Fatalf("unexpected opening history: %+v, %v", got, err)
	}
	if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
		t.Fatal("listing opening history changed ledger state")
	}

	move(t, ctx, transfers, "history-fund", "system-reservoir", "alice", 250)
	before = ledgerContents(t, ctx, conn)
	got, err = history.New(store).List(ctx)
	if err != nil || len(got) != 2 || got[0].ID != "system-opening" ||
		len(got[1].Entries) != 2 ||
		got[1].Entries[0].AccountID != "system-reservoir" ||
		got[1].Entries[0].Amount.Sititos != -250 ||
		got[1].Entries[1].AccountID != "alice" ||
		got[1].Entries[1].Amount.Sititos != 250 ||
		got[1].Entries[0].ID >= got[1].Entries[1].ID {
		t.Fatalf("unexpected transfer history: %+v, %v", got, err)
	}
	if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
		t.Fatal("listing transfer history changed ledger state")
	}
}
