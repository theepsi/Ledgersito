//go:build integration && initializationcheck

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
)

func initializationDB(t *testing.T, populated bool) (*pgx.Conn, context.Context) {
	t.Helper()
	conn, ctx := testDB(t)
	schema := "init_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("remove isolated initialization schema: %v", err)
		}
	})
	config := conn.Config()
	config.RuntimeParams["search_path"] = schema
	scoped, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := scoped.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	for _, file := range []string{
		"00001_create_accounts.sql", "00002_account_names_and_replays.sql", "00003_ledger_initialization.sql",
	} {
		if populated && file == "00003_ledger_initialization.sql" {
			if _, err := scoped.Exec(ctx, "INSERT INTO accounts (id, name, balance) VALUES ('legacy', 'Preserved name', 42)"); err != nil {
				t.Fatal(err)
			}
		}
		content, err := os.ReadFile("../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(content), "-- +goose Down")
		if _, err := scoped.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s in isolated schema: %v", file, err)
		}
	}
	return scoped, ctx
}

func TestInitializationIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	if _, err := conn.Exec(ctx, "INSERT INTO accounts (id, name) VALUES ('existing', 'Savings')"); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewAPIStore(conn.Config())
	created, err := store.Initialize(ctx)
	if err != nil || !created {
		t.Fatalf("initialization: %t %v", created, err)
	}
	created, err = store.Initialize(ctx)
	if err != nil || created {
		t.Fatalf("repeat: %t %v", created, err)
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM ledger_transactions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("opening count: %d %v", count, err)
	}
	var total int64
	if err := conn.QueryRow(ctx, "SELECT sum(amount)::bigint FROM entries").Scan(&total); err != nil || total != 0 {
		t.Fatalf("entry sum: %d %v", total, err)
	}
	rows, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected existing account plus system accounts, got %d", len(rows))
	}
	for _, a := range rows {
		switch a.ID {
		case "existing":
			if a.Role != "ordinary" || a.Balance.Sititos != 0 || a.Name != "Savings" {
				t.Fatal(a)
			}
		case "system-origin":
			if a.Role != "origin" || a.Balance.Sititos != -100000000 {
				t.Fatal(a)
			}
		case "system-reservoir":
			if a.Role != "reservoir" || a.Balance.Sititos != 100000000 {
				t.Fatal(a)
			}
		default:
			t.Fatal(a)
		}
	}
	generator := &fixtureGenerator{t: t, conn: conn, prefix: "roles-" + rand.Text()}
	handler := httpapi.New(accounts.New(generator, store), slog.New(slog.NewTextHandler(io.Discard, nil)))
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, httptest.NewRequest("GET", "/accounts", nil))
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"role":"origin"`) ||
		!strings.Contains(listed.Body.String(), `"role":"reservoir"`) ||
		!strings.Contains(listed.Body.String(), `"role":"ordinary"`) {
		t.Fatalf("GET does not expose roles: %d %s", listed.Code, listed.Body)
	}

	t.Run("valid redistribution is not a new initialization", func(t *testing.T) {
		_, err := conn.Exec(ctx, `
BEGIN;
INSERT INTO ledger_transactions (id, reference) VALUES ('redistribution', 'fixture transfer');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
('redistribution', 'system-reservoir', -123), ('redistribution', 'existing', 123);
UPDATE accounts SET balance = balance - 123 WHERE id = 'system-reservoir';
UPDATE accounts SET balance = balance + 123 WHERE id = 'existing';
UPDATE ledger_transactions SET posted = true WHERE id = 'redistribution';
COMMIT;`)
		if err != nil {
			t.Fatal(err)
		}
		if created, err := store.Initialize(ctx); err != nil || created {
			t.Fatalf("repeat after valid transfer: %t %v", created, err)
		}
	})

	t.Run("inconsistent balances rejected without repair", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE accounts SET balance = 124 WHERE id = 'existing'"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Initialize(ctx); err == nil {
			t.Fatal("corrupt balance accepted")
		}
		var balance int64
		if err := conn.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 'existing'").Scan(&balance); err != nil || balance != 124 {
			t.Fatal("failed initialization repaired or changed balance")
		}
	})
}

func TestConcurrentInitializationIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	store := postgres.NewAPIStore(conn.Config())
	start := make(chan struct{})
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 10)
	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			<-start
			created, err := store.Initialize(ctx)
			results <- result{created, err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	created := 0
	for r := range results {
		if r.err != nil {
			t.Error(r.err)
		}
		if r.created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("got %d initializations", created)
	}
	for table, want := range map[string]int{"accounts": 2, "ledger_transactions": 1, "entries": 2, "ledger_initialization": 1} {
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != want {
			t.Fatalf("unexpected %s count: %d %v", table, count, err)
		}
	}
}

func TestInitializationRollbackIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	// Force the last write to fail, after accounts and opening entries exist.
	if _, err := conn.Exec(ctx, "ALTER TABLE ledger_initialization ADD CONSTRAINT injected_failure CHECK (supply = 0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewAPIStore(conn.Config()).Initialize(ctx); err == nil {
		t.Fatal("expected failure")
	}
	for _, table := range []string{"accounts", "ledger_transactions", "entries", "ledger_initialization"} {
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s state: %d %v", table, count, err)
		}
	}
}

func TestHistoryProtectionIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	if _, err := postgres.NewAPIStore(conn.Config()).Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE entries SET amount = amount + 1",
		"DELETE FROM entries",
		"TRUNCATE entries",
		"DELETE FROM ledger_transactions",
		"UPDATE ledger_transactions SET reference = 'changed'",
		"UPDATE accounts SET role = 'ordinary' WHERE role = 'origin'",
		"UPDATE accounts SET balance = -99999999 WHERE role = 'origin'",
		"UPDATE accounts SET balance = -1 WHERE role = 'reservoir'",
		"DELETE FROM ledger_initialization",
		"INSERT INTO entries (transaction_id, account_id, amount) VALUES ('system-opening', 'system-origin', -1)",
		"INSERT INTO ledger_transactions (id, reference, posted) VALUES ('empty', 'invalid', true)",
	} {
		if _, err := conn.Exec(ctx, statement); err == nil {
			t.Errorf("mutation accepted: %s", statement)
		}
	}
	if _, err := conn.Exec(ctx, "INSERT INTO accounts (id, name) VALUES ('recipient', 'Recipient')"); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO ledger_transactions (id, reference) VALUES ('unbalanced', 'invalid');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
('unbalanced', 'system-reservoir', -10), ('unbalanced', 'recipient', 9);
UPDATE ledger_transactions SET posted = true WHERE id = 'unbalanced';`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("unbalanced transaction committed")
	}
}

func TestUnexplainedOpeningBalanceIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	if _, err := conn.Exec(ctx, "INSERT INTO accounts (id, name, balance) VALUES ('unexpected', 'Savings', 10)"); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewAPIStore(conn.Config()).Initialize(ctx); err == nil {
		t.Fatal("unexplained funds accepted")
	}
}

func TestOldReplayRemainsUnchangedIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	body := []byte(`{"id":"old","name":"Savings","balance":{"sititos":0,"currency":"SITO"}}`)
	if _, err := conn.Exec(ctx, "INSERT INTO account_creation_replays (key, request_name, status_code, response_body) VALUES ('old-key', 'Savings', 201, $1)", body); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewAPIStore(conn.Config()).Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := dbsql.New(conn).GetCreation(ctx, "old-key")
	if err != nil || string(got.ResponseBody) != string(body) {
		t.Fatalf("old response changed: %v", err)
	}
	replayed, err := postgres.NewAPIStore(conn.Config()).CreateOnce(ctx, "old-key", "Savings", func(createaccount.Store) (accounts.Response, error) {
		return accounts.Response{}, errors.New("old replay attempted new account creation")
	})
	if err != nil || replayed.Status != 201 || string(replayed.Body) != string(body) {
		t.Fatalf("old replay not preserved: %+v %v", replayed, err)
	}
}

func TestRoleMigrationIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, true)
	var name, role string
	var balance int64
	if err := conn.QueryRow(ctx, "SELECT name, role, balance FROM accounts WHERE id = 'legacy'").Scan(&name, &role, &balance); err != nil {
		t.Fatal(err)
	}
	if name != "Preserved name" || role != "ordinary" || balance != 42 {
		t.Fatalf("role migration changed existing account: %s %s %d", name, role, balance)
	}
}
