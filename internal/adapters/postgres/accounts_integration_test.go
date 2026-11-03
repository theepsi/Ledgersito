//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

func testDB(t *testing.T) (*pgx.Conn, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is required; prepare the ledger_test database first")
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}
	if config.Database != "ledger_test" {
		t.Fatal("refusing test database other than ledger_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if err := conn.Close(closeCtx); err != nil {
			t.Errorf("close test connection: %v", err)
		}
	})
	var database string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		t.Fatalf("verify database: %v", err)
	}
	if database != "ledger_test" {
		t.Fatal("connected to unexpected database; refusing writes")
	}
	return conn, ctx
}

func fixture(t *testing.T, conn *pgx.Conn, id string) account.Account {
	t.Helper()
	a, err := account.New(id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(ctx, "DELETE FROM accounts WHERE id = $1", id); err != nil {
			t.Errorf("remove test fixture: %v", err)
		}
	})
	return a
}

func TestAccountStorageIntegration(t *testing.T) {
	conn, ctx := testDB(t)
	reader, _ := testDB(t)
	store := postgres.NewAccountStore(conn)
	prefix := "test-" + rand.Text()

	t.Run("persist exact IDs and zero balances", func(t *testing.T) {
		for _, suffix := range []string{"-A", "-a", "-\u00e9"} {
			a := fixture(t, conn, prefix+suffix)
			if err := store.Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			var id string
			var balance int64
			if err := reader.QueryRow(ctx, "SELECT id, balance FROM accounts WHERE id = $1", a.ID()).Scan(&id, &balance); err != nil {
				t.Fatal(err)
			}
			if id != a.ID() || balance != 0 {
				t.Fatalf("got ID %q and balance %d", id, balance)
			}
		}
	})

	t.Run("duplicate preserves existing balance", func(t *testing.T) {
		a := fixture(t, conn, prefix+"-duplicate")
		if err := store.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "UPDATE accounts SET balance = 123 WHERE id = $1", a.ID()); err != nil {
			t.Fatal(err)
		}
		if err := store.Create(ctx, a); !errors.Is(err, createaccount.ErrAlreadyExists) {
			t.Fatalf("expected conflict, got %v", err)
		}
		var balance int64
		if err := reader.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = $1", a.ID()).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != 123 {
			t.Fatal("duplicate creation changed existing balance")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		a := fixture(t, conn, prefix+"-canceled")
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if err := store.Create(canceled, a); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	})

	t.Run("closed database connection", func(t *testing.T) {
		closed, _ := testDB(t)
		a := fixture(t, conn, prefix+"-closed")
		if err := closed.Close(ctx); err != nil {
			t.Fatal(err)
		}
		err := postgres.NewAccountStore(closed).Create(ctx, a)
		if err == nil || errors.Is(err, createaccount.ErrAlreadyExists) {
			t.Fatalf("expected a non-conflict connection error, got %v", err)
		}
	})

	t.Run("concurrent duplicate creation", func(t *testing.T) {
		a := fixture(t, conn, prefix+"-concurrent")
		const attempts = 10
		connections := make([]*pgx.Conn, attempts)
		for i := range connections {
			connections[i], _ = testDB(t)
		}
		results := make(chan error, attempts)
		start := make(chan struct{})
		var workers sync.WaitGroup
		for _, connection := range connections {
			workers.Go(func() {
				<-start
				results <- postgres.NewAccountStore(connection).Create(ctx, a)
			})
		}
		close(start)
		workers.Wait()
		close(results)
		successes, conflicts := 0, 0
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, createaccount.ErrAlreadyExists):
				conflicts++
			default:
				t.Errorf("unexpected insert error: %v", err)
			}
		}
		if successes != 1 || conflicts != attempts-1 {
			t.Errorf("got %d successes and %d conflicts", successes, conflicts)
		}
		var count int
		if err := reader.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE id = $1", a.ID()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("got %d rows, want 1", count)
		}
	})
}
