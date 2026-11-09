//go:build integration && initializationcheck

package postgres_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/reconciliation"
)

func ledgerContents(t *testing.T, ctx context.Context, conn *pgx.Conn) []string {
	t.Helper()
	var result []string
	for _, table := range []string{"accounts", "ledger_transactions", "entries", "ledger_initialization", "account_creation_replays", "transfer_replays"} {
		var rows string
		err := conn.QueryRow(ctx, "SELECT coalesce(jsonb_agg(row_data ORDER BY row_data::text), '[]'::jsonb)::text FROM (SELECT to_jsonb(t) row_data FROM "+
			pgx.Identifier{table}.Sanitize()+" t) rows").Scan(&rows)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, rows)
	}
	return result
}

func TestReconciliationLifecycleIntegration(t *testing.T) {
	t.Run("empty ledger", func(t *testing.T) {
		conn, ctx := initializationDB(t, false)
		before := ledgerContents(t, ctx, conn)
		report, err := postgres.NewAPIStore(conn.Config()).Reconcile(ctx)
		if err != nil || report.Initialized || len(report.Findings) != 0 {
			t.Fatalf("unexpected empty-ledger report: %+v %v", report, err)
		}
		if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
			t.Fatal("reconciliation changed empty ledger")
		}
	})
	conn, ctx, service := transferDB(t, false)
	store := postgres.NewAPIStore(conn.Config())
	check := func(initialized bool) {
		t.Helper()
		before := ledgerContents(t, ctx, conn)
		report, err := store.Reconcile(ctx)
		if err != nil || report.Initialized != initialized || len(report.Findings) != 0 {
			t.Fatalf("unexpected reconciliation: %+v %v", report, err)
		}
		if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
			t.Fatal("reconciliation changed persisted state")
		}
	}
	check(false)
	if _, err := store.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	check(true)
	move(t, ctx, service, "fund", "system-reservoir", "alice", 250)
	move(t, ctx, service, "circulate", "alice", "bob", 50)
	if _, err := accounts.New(transferIDs{}, store).Create(ctx, "new-account", "Savings"); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestReconciliationCorruptionIntegration(t *testing.T) {
	for _, tt := range []struct {
		name string
		sql  string
		want []string
	}{
		{"cached balance", "UPDATE accounts SET balance = 1 WHERE id = 'alice'",
			[]string{"account_balance_mismatch", "circulating_supply_mismatch", "total_balance_mismatch"}},
		{"missing initialization", "ALTER TABLE ledger_initialization DISABLE TRIGGER USER; DELETE FROM ledger_initialization",
			[]string{"initialization_record"}},
		{"metadata", "ALTER TABLE ledger_initialization DISABLE TRIGGER USER; ALTER TABLE ledger_initialization DROP CONSTRAINT ledger_initialization_supply_check; UPDATE ledger_initialization SET supply = 1",
			[]string{"initialization_record"}},
		{"opening reference", "ALTER TABLE ledger_transactions DISABLE TRIGGER USER; UPDATE ledger_transactions SET reference = 'wrong' WHERE id = 'system-opening'",
			[]string{"opening_history"}},
		{"system identity", "ALTER TABLE entries DROP CONSTRAINT entries_account_id_fkey; ALTER TABLE accounts DISABLE TRIGGER USER; UPDATE accounts SET id = 'wrong-origin' WHERE id = 'system-origin'",
			[]string{"system_origin", "entry_account_missing", "account_balance_mismatch"}},
		{"system name", "UPDATE accounts SET name = 'Wrong' WHERE id = 'system-reservoir'",
			[]string{"system_reservoir"}},
		{"system role", "ALTER TABLE accounts DISABLE TRIGGER USER; UPDATE accounts SET role = 'ordinary' WHERE id = 'system-reservoir'",
			[]string{"system_reservoir"}},
		{"unknown role", "ALTER TABLE accounts DROP CONSTRAINT accounts_role; ALTER TABLE accounts DISABLE TRIGGER USER; UPDATE accounts SET role = 'unknown' WHERE id = 'alice'",
			[]string{"account_role"}},
		{"unposted empty transaction", "ALTER TABLE ledger_transactions DISABLE TRIGGER USER; INSERT INTO ledger_transactions (id, reference) VALUES ('broken', 'fixture')",
			[]string{"transaction_unposted", "transaction_entry_count"}},
		{"unbalanced opening", "ALTER TABLE entries DISABLE TRIGGER USER; UPDATE entries SET amount = 99999999 WHERE account_id = 'system-reservoir'",
			[]string{"opening_history", "transaction_unbalanced", "account_balance_mismatch"}},
		{"negative balance", "ALTER TABLE accounts DROP CONSTRAINT accounts_nonnegative; UPDATE accounts SET balance = -1 WHERE id = 'alice'",
			[]string{"non_origin_negative", "account_balance_mismatch", "circulating_supply_mismatch"}},
		{"origin balance", "ALTER TABLE accounts DISABLE TRIGGER USER; UPDATE accounts SET balance = -99999999 WHERE id = 'system-origin'",
			[]string{"origin_balance", "account_balance_mismatch", "total_balance_mismatch"}},
		{"reservoir receives and origin participates", `
ALTER TABLE entries DISABLE TRIGGER USER;
INSERT INTO ledger_transactions (id, reference) VALUES ('invalid-transfer', 'fixture');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
('invalid-transfer', 'system-reservoir', 1), ('invalid-transfer', 'system-origin', -1);
UPDATE ledger_transactions SET posted = true WHERE id = 'invalid-transfer'`,
			[]string{"reservoir_credit", "origin_entry_outside_opening"}},
		{"dangling transaction", "ALTER TABLE entries DROP CONSTRAINT entries_transaction_id_fkey; ALTER TABLE entries DISABLE TRIGGER USER; UPDATE entries SET transaction_id = 'missing' WHERE account_id = 'system-origin'",
			[]string{"entry_transaction_missing", "opening_history", "transaction_unbalanced"}},
		{"missing opening reference", "ALTER TABLE ledger_initialization DROP CONSTRAINT ledger_initialization_opening_transaction_id_fkey; ALTER TABLE ledger_initialization DISABLE TRIGGER USER; UPDATE ledger_initialization SET opening_transaction_id = 'missing'",
			[]string{"initialization_record", "initialization_opening_missing"}},
		{"aggregate exceeds int64", `
ALTER TABLE entries DISABLE TRIGGER USER;
INSERT INTO ledger_transactions (id, reference) VALUES ('large', 'fixture');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
('large', 'alice', 9223372036854775807), ('large', 'alice', 9223372036854775807),
('large', 'bob', -9223372036854775807), ('large', 'bob', -9223372036854775807);
UPDATE ledger_transactions SET posted = true WHERE id = 'large'`,
			[]string{"account_balance_mismatch"}},
		{"balance total exceeds int64", "UPDATE accounts SET balance = 9223372036854775807 WHERE id IN ('alice', 'bob')",
			[]string{"account_balance_mismatch", "circulating_supply_mismatch", "total_balance_mismatch"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn, ctx, _ := transferDB(t, true)
			// Tampering is confined to this test's uniquely named disposable schema.
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, tt.sql); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			store := postgres.NewAPIStore(conn.Config())
			before := ledgerContents(t, ctx, conn)
			report, err := store.Reconcile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			codes := make(map[string]bool)
			for i, f := range report.Findings {
				codes[f.Code] = true
				if i > 0 {
					prior := report.Findings[i-1]
					if prior.Code > f.Code || (prior.Code == f.Code && prior.ID > f.ID) {
						t.Fatalf("findings not deterministically ordered: %+v", report.Findings)
					}
				}
			}
			for _, code := range tt.want {
				if !codes[code] {
					t.Errorf("missing %s: %+v", code, report.Findings)
				}
			}
			again, err := store.Reconcile(ctx)
			if err != nil || !reflect.DeepEqual(report, again) || !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
				t.Fatalf("report changed or reconciliation wrote data: %+v %v", again, err)
			}
		})
	}
}

func TestReconciliationPartialInitializationIntegration(t *testing.T) {
	for _, statement := range []string{
		"UPDATE accounts SET balance = 1 WHERE id = 'alice'",
		"INSERT INTO accounts (id, name) VALUES ('system-origin', 'Reserved ID')",
		"INSERT INTO accounts (id, name, role) VALUES ('unexpected', 'Unexpected', 'reservoir')",
	} {
		t.Run(statement, func(t *testing.T) {
			conn, ctx, _ := transferDB(t, false)
			if _, err := conn.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			report, err := postgres.NewAPIStore(conn.Config()).Reconcile(ctx)
			if err != nil || report.Initialized || len(report.Findings) == 0 {
				t.Fatalf("partial state treated as uninitialized: %+v %v", report, err)
			}
		})
	}
}

type reconciliationTraceKey struct{}

type reconciliationTracer struct {
	afterState func(*pgx.Conn)
}

func (tr *reconciliationTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, reconciliationTraceKey{}, strings.HasPrefix(data.SQL, "-- name: ReconciliationState "))
}

func (tr *reconciliationTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if state, _ := ctx.Value(reconciliationTraceKey{}).(bool); state && data.Err == nil {
		tr.afterState(conn)
	}
}

func TestReconciliationSnapshotIntegration(t *testing.T) {
	conn, ctx, service := transferDB(t, true)
	config := conn.Config()
	config.Tracer = &reconciliationTracer{afterState: func(reader *pgx.Conn) {
		var isolation, readOnly string
		if err := reader.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
			t.Fatal(err)
		}
		if err := reader.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil {
			t.Fatal(err)
		}
		if isolation != "repeatable read" || readOnly != "on" {
			t.Fatalf("not a read-only consistent snapshot: %s %s", isolation, readOnly)
		}
		move(t, ctx, service, "concurrent-fund", "system-reservoir", "alice", 1)
		// Commit a mismatch after the reader's snapshot to prove it is retained.
		if _, err := conn.Exec(ctx, "UPDATE accounts SET balance = 2 WHERE id = 'alice'"); err != nil {
			t.Fatal(err)
		}
	}}
	report, err := postgres.NewAPIStore(config).Reconcile(ctx)
	if err != nil || !report.Initialized || len(report.Findings) != 0 {
		t.Fatalf("mixed-snapshot false finding: %+v %v", report, err)
	}
	latest, err := postgres.NewAPIStore(conn.Config()).Reconcile(ctx)
	if err != nil || len(latest.Findings) == 0 {
		t.Fatalf("new run did not observe committed mismatch: %+v %v", latest, err)
	}
}

func TestReconciliationOperationalFailureIntegration(t *testing.T) {
	conn, ctx, _ := transferDB(t, true)
	config := conn.Config()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	config.Tracer = &reconciliationTracer{afterState: func(*pgx.Conn) { cancel() }}
	report, err := reconciliation.New(postgres.NewAPIStore(config)).Execute(cancelled)
	if err == nil || !reflect.DeepEqual(report, reconciliation.Report{}) {
		t.Fatalf("operational failure returned a consistency report: %+v %v", report, err)
	}
	config = conn.Config()
	config.RuntimeParams["search_path"] = "missing_" + fmt.Sprint(conn.PgConn().PID())
	report, err = reconciliation.New(postgres.NewAPIStore(config)).Execute(ctx)
	if err == nil || !reflect.DeepEqual(report, reconciliation.Report{}) {
		t.Fatalf("missing schema accepted: %+v %v", report, err)
	}
}
