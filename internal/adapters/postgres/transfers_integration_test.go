//go:build integration && initializationcheck

package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

type transferIDs struct{}

func (transferIDs) Generate() (string, error) { return rand.Text(), nil }

func transferDB(t *testing.T, initialized bool) (*pgx.Conn, context.Context, *transfers.Service) {
	t.Helper()
	conn, ctx := initializationDB(t, false)
	if _, err := conn.Exec(ctx, "INSERT INTO accounts (id, name) VALUES ('alice', 'Alice'), ('bob', 'Bob'), ('charlie', 'Charlie')"); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewAPIStore(conn.Config())
	if initialized {
		if _, err := store.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return conn, ctx, transfers.New(transferIDs{}, store)
}

func move(t *testing.T, ctx context.Context, s *transfers.Service, key, from, to string, amount int64) transfers.Response {
	t.Helper()
	reply, err := s.Execute(ctx, key, transfer.Request{SourceID: from, DestinationID: to, Sititos: amount})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Status != 201 {
		t.Fatalf("unexpected status: %d", reply.Status)
	}
	return reply
}

func checkTransferLedger(t *testing.T, conn *pgx.Conn, ctx context.Context, transactions int) {
	t.Helper()
	var valid bool
	err := conn.QueryRow(ctx, `
SELECT
    (SELECT count(*) FROM ledger_transactions) = $1 AND
    (SELECT count(*) FROM entries) = $1 * 2 AND
    (SELECT sum(balance) FROM accounts WHERE role <> 'origin') = 100000000 AND
    (SELECT sum(balance) FROM accounts) = 0 AND
    NOT EXISTS (SELECT 1 FROM accounts WHERE role <> 'origin' AND balance < 0) AND
    NOT EXISTS (
        SELECT 1 FROM accounts a LEFT JOIN
        (SELECT account_id, sum(amount) total FROM entries GROUP BY account_id) e ON e.account_id = a.id
        WHERE a.balance <> coalesce(e.total, 0)
    ) AND NOT EXISTS (
        SELECT 1 FROM ledger_transactions t LEFT JOIN entries e ON e.transaction_id = t.id
        GROUP BY t.id, t.posted HAVING NOT t.posted OR count(e.id) <> 2 OR sum(e.amount) <> 0
    )`, transactions).Scan(&valid)
	if err != nil || !valid {
		t.Fatalf("ledger invariants failed: valid=%t, %v", valid, err)
	}
}

func TestTransferLifecycleIntegration(t *testing.T) {
	conn, ctx, service := transferDB(t, true)
	req := transfer.Request{SourceID: "system-reservoir", DestinationID: "alice", Sititos: 250}
	first := move(t, ctx, service, "fund", req.SourceID, req.DestinationID, req.Sititos)
	move(t, ctx, service, "spend", "alice", "bob", 250)
	replay, err := service.Execute(ctx, "fund", req)
	if err != nil || replay.Status != first.Status || string(replay.Body) != string(first.Body) {
		t.Fatalf("response loss replay changed: %+v %v", replay, err)
	}
	for _, changed := range []transfer.Request{
		{SourceID: "bob", DestinationID: "alice", Sititos: 250},
		{SourceID: req.SourceID, DestinationID: "bob", Sititos: 250},
		{SourceID: req.SourceID, DestinationID: req.DestinationID, Sititos: 251},
	} {
		if _, err := service.Execute(ctx, "fund", changed); !errors.Is(err, transfers.ErrKeyReused) {
			t.Fatalf("expected key conflict: %v", err)
		}
	}
	store := postgres.NewAPIStore(conn.Config())
	// Account creation and transfers may deliberately use the same key.
	if _, err := accounts.New(transferIDs{}, store).Create(ctx, "fund", "Separate scope"); err != nil {
		t.Fatal(err)
	}
	if created, err := store.Initialize(ctx); err != nil || created {
		t.Fatalf("repeat initialization after transfers: %t %v", created, err)
	}
	checkTransferLedger(t, conn, ctx, 3)
	var balance int64
	if err := conn.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 'alice'").Scan(&balance); err != nil || balance != 0 {
		t.Fatalf("full-balance spend failed: %d %v", balance, err)
	}
}

func TestTransferBusinessErrorsIntegration(t *testing.T) {
	t.Run("initialization precedes account existence", func(t *testing.T) {
		conn, ctx, service := transferDB(t, false)
		_, err := service.Execute(ctx, "uninitialized", transfer.Request{SourceID: "missing", DestinationID: "alice", Sititos: 1})
		if !errors.Is(err, transfers.ErrNotInitialized) {
			t.Fatal(err)
		}
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM transfer_replays").Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed key survived: %d %v", count, err)
		}
	})
	conn, ctx, service := transferDB(t, true)
	for _, tt := range []struct {
		name, from, to string
		amount         int64
		err            error
	}{
		{"missing source", "missing", "alice", 1, transfers.ErrAccountNotFound},
		{"missing destination", "alice", "missing", 1, transfers.ErrAccountNotFound},
		{"existence before roles", "system-origin", "missing", 1, transfers.ErrAccountNotFound},
		{"origin source", "system-origin", "alice", 1, transfer.ErrNotTransferable},
		{"origin destination", "alice", "system-origin", 1, transfer.ErrNotTransferable},
		{"reservoir destination before funds", "alice", "system-reservoir", 1, transfer.ErrNotTransferable},
		{"insufficient", "alice", "bob", 1, transfer.ErrInsufficientFunds},
		{"same", "alice", "alice", 1, transfer.ErrSameAccount},
		{"zero", "alice", "bob", 0, transfer.ErrInvalidAmount},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Execute(ctx, "failure", transfer.Request{SourceID: tt.from, DestinationID: tt.to, Sititos: tt.amount})
			if !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
			checkTransferLedger(t, conn, ctx, 1)
			var count int
			if err := conn.QueryRow(ctx, "SELECT count(*) FROM transfer_replays").Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed transfer reserved key: %d %v", count, err)
			}
		})
	}
	move(t, ctx, service, "fund", "system-reservoir", "alice", 1)
	move(t, ctx, service, "failure", "alice", "bob", 1)
	checkTransferLedger(t, conn, ctx, 3)
}

func TestTransferConcurrencyIntegration(t *testing.T) {
	for _, mode := range []string{"same key", "same key conflict", "overspend", "opposite directions"} {
		t.Run(mode, func(t *testing.T) {
			conn, ctx, service := transferDB(t, true)
			move(t, ctx, service, "fund-alice", "system-reservoir", "alice", 100)
			wantTransactions := 3
			requests := []transfer.Request{
				{SourceID: "alice", DestinationID: "bob", Sititos: 80},
				{SourceID: "alice", DestinationID: "charlie", Sititos: 80},
			}
			keys := []string{"one", "two"}
			switch mode {
			case "same key":
				keys[1] = keys[0]
				requests[1] = requests[0]
			case "same key conflict":
				keys[1] = keys[0]
			case "opposite directions":
				move(t, ctx, service, "fund-bob", "system-reservoir", "bob", 100)
				requests[1] = transfer.Request{SourceID: "bob", DestinationID: "alice", Sititos: 80}
				wantTransactions = 5
			}
			type result struct {
				reply transfers.Response
				err   error
			}
			start := make(chan struct{})
			results := make(chan result, 2)
			for i := range requests {
				go func(i int) {
					<-start
					reply, err := service.Execute(ctx, keys[i], requests[i])
					results <- result{reply, err}
				}(i)
			}
			close(start)
			first, second := <-results, <-results
			switch mode {
			case "same key":
				if first.err != nil || second.err != nil || string(first.reply.Body) != string(second.reply.Body) {
					t.Fatalf("same key duplicated/failed: %+v %+v", first, second)
				}
			case "opposite directions":
				if first.err != nil || second.err != nil {
					t.Fatalf("opposite transfers failed: %v %v", first.err, second.err)
				}
			default:
				wantErr := transfer.ErrInsufficientFunds
				if mode == "same key conflict" {
					wantErr = transfers.ErrKeyReused
				}
				successes := 0
				for _, err := range []error{first.err, second.err} {
					if err == nil {
						successes++
					} else if !errors.Is(err, wantErr) {
						t.Fatalf("expected %v, got %v", wantErr, err)
					}
				}
				if successes != 1 {
					t.Fatalf("expected one success, got %d", successes)
				}
			}
			checkTransferLedger(t, conn, ctx, wantTransactions)
		})
	}
}

func TestTransferPartialWriteRollbackIntegration(t *testing.T) {
	for _, phase := range []string{"entry", "balance", "posting", "replay"} {
		t.Run(phase, func(t *testing.T) {
			conn, ctx, service := transferDB(t, true)
			statement := ""
			switch phase {
			case "entry":
				statement = "ALTER TABLE entries ADD CONSTRAINT injected_failure CHECK (account_id <> 'alice')"
			case "balance":
				statement = "ALTER TABLE accounts ADD CONSTRAINT injected_failure CHECK (id <> 'alice' OR balance = 0)"
			case "posting":
				statement = "ALTER TABLE ledger_transactions ADD CONSTRAINT injected_failure CHECK (NOT posted OR id = 'system-opening')"
			case "replay":
				statement = "ALTER TABLE transfer_replays ADD CONSTRAINT injected_failure CHECK (status_code IS NULL)"
			}
			if _, err := conn.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			req := transfer.Request{SourceID: "system-reservoir", DestinationID: "alice", Sititos: 25}
			if _, err := service.Execute(ctx, "retry", req); err == nil {
				t.Fatal("injected failure accepted")
			}
			checkTransferLedger(t, conn, ctx, 1)
			var count int
			if err := conn.QueryRow(ctx, "SELECT count(*) FROM transfer_replays").Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial replay survived: %d %v", count, err)
			}
			table := map[string]string{"entry": "entries", "balance": "accounts", "posting": "ledger_transactions", "replay": "transfer_replays"}[phase]
			if _, err := conn.Exec(ctx, "ALTER TABLE "+pgx.Identifier{table}.Sanitize()+" DROP CONSTRAINT injected_failure"); err != nil {
				t.Fatal(err)
			}
			move(t, ctx, service, "retry", req.SourceID, req.DestinationID, req.Sititos)
			checkTransferLedger(t, conn, ctx, 2)
		})
	}
}

func waitTransferLock(t *testing.T, conn *pgx.Conn, ctx context.Context, application string) {
	t.Helper()
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := conn.QueryRow(wait, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1 AND wait_event_type = 'Lock')", application).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-wait.Done():
			t.Fatal("transfer never reached lock wait")
		case <-ticker.C:
		}
	}
}

func TestTransferWaitingKeyRollbackIntegration(t *testing.T) {
	conn, ctx, _ := transferDB(t, true)
	// Observe in autocommit mode; the owner transaction caches activity snapshots.
	observer, _ := testDB(t)
	owner := postgres.NewAPIStore(conn.Config())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(ctx, "INSERT INTO transfer_replays (key, source_account_id, destination_account_id, amount) VALUES ('waiting', 'system-reservoir', 'alice', 25)"); err != nil {
		t.Fatal(err)
	}
	config := conn.Config()
	config.RuntimeParams["application_name"] = "waiting_" + rand.Text()
	service := transfers.New(transferIDs{}, postgres.NewAPIStore(config))
	request, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var workerErr error
	go func() {
		defer close(done)
		_, workerErr = service.Execute(request, "waiting", transfer.Request{SourceID: "system-reservoir", DestinationID: "bob", Sititos: 30})
	}()
	defer func() {
		cancel()
		<-done
	}()
	waitTransferLock(t, observer, ctx, config.RuntimeParams["application_name"])
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	if workerErr != nil {
		t.Fatalf("rolled-back key not acquired: %v", workerErr)
	}
	checkTransferLedger(t, conn, ctx, 2)
	if _, err := owner.TransferOnce(ctx, "waiting", transfer.Request{SourceID: "system-reservoir", DestinationID: "bob", Sititos: 30}, func(transfers.Store) (transfers.Response, error) {
		t.Error("replay attempted another transfer")
		return transfers.Response{}, errors.New("unexpected callback")
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTransferLockTimeoutIntegration(t *testing.T) {
	for _, mode := range []string{"account", "key"} {
		t.Run(mode, func(t *testing.T) {
			conn, ctx, service := transferDB(t, true)
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Error(err)
				}
			}()
			query := "SELECT id FROM accounts WHERE id = 'system-reservoir' FOR UPDATE"
			if mode == "key" {
				query = "INSERT INTO transfer_replays (key, source_account_id, destination_account_id, amount) VALUES ('timeout', 'system-reservoir', 'alice', 25)"
			}
			if _, err := tx.Exec(ctx, query); err != nil {
				t.Fatal(err)
			}
			request, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()
			h := httpapi.NewRouter(accounts.New(transferIDs{}, postgres.NewAPIStore(conn.Config())), service, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/transfers", strings.NewReader(`{"source_account_id":"system-reservoir","destination_account_id":"alice","amount_sititos":25}`)).WithContext(request)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "timeout")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 503 || !strings.Contains(w.Body.String(), `"request_timeout"`) {
				t.Fatalf("lock timeout: %d %s", w.Code, w.Body)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			move(t, ctx, service, "timeout", "system-reservoir", "alice", 25)
			checkTransferLedger(t, conn, ctx, 2)
		})
	}
}

func TestTransferHTTPIntegration(t *testing.T) {
	conn, ctx, service := transferDB(t, true)
	h := httpapi.NewRouter(accounts.New(transferIDs{}, postgres.NewAPIStore(conn.Config())), service, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "/transfers", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "http")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"source_account_id":"system-reservoir","destination_account_id":"alice","amount_sititos":25}`
	first := post(body)
	if first.Code != 201 {
		t.Fatalf("HTTP transfer: %d %s", first.Code, first.Body)
	}
	move(t, ctx, service, "later", "alice", "bob", 25)
	replay := post(body)
	if replay.Code != 201 || replay.Body.String() != first.Body.String() {
		t.Fatal("HTTP replay changed")
	}
	conflict := post(strings.Replace(body, ":25", ":26", 1))
	if conflict.Code != 422 || !strings.Contains(conflict.Body.String(), `"idempotency_key_reused"`) {
		t.Fatal("HTTP key conflict missing")
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var posted bool
	if err := conn.QueryRow(ctx, "SELECT posted FROM ledger_transactions WHERE id = $1", result.ID).Scan(&posted); err != nil || !posted {
		t.Fatalf("response ID does not identify committed transaction: %s %v", result.ID, err)
	}
	checkTransferLedger(t, conn, ctx, 3)
}

func TestTransferReservoirEntryGuardIntegration(t *testing.T) {
	conn, ctx, service := transferDB(t, true)
	move(t, ctx, service, "fund", "system-reservoir", "alice", 25)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(ctx, "INSERT INTO ledger_transactions (id, reference) VALUES ('return', '')"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO entries (transaction_id, account_id, amount) VALUES ('return', 'system-reservoir', 1)"); err == nil {
		t.Fatal("database accepted reservoir replenishment")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	checkTransferLedger(t, conn, ctx, 2)
}
