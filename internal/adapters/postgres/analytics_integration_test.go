//go:build integration && initializationcheck

package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/analytics"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

func TestAnalyticsUninitializedIntegration(t *testing.T) {
	conn, ctx, _ := transferDB(t, false)
	before := ledgerContents(t, ctx, conn)
	_, err := analytics.New(postgres.NewAPIStore(conn.Config())).Execute(ctx, nil)
	if !errors.Is(err, analytics.ErrNotInitialized) {
		t.Fatalf("got %v, want uninitialized error", err)
	}
	if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
		t.Fatal("analytics changed an uninitialized ledger")
	}
}

func TestAnalyticsAggregateIntegration(t *testing.T) {
	conn, ctx, transferService := transferDB(t, true)
	service := analytics.New(postgres.NewAPIStore(conn.Config()))
	initial, err := service.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Transfers.Count != 0 || initial.Transfers.TotalVolume.Sititos != "0" ||
		initial.Transfers.ReservoirDistribution.Count != 0 ||
		initial.Transfers.OrdinaryToOrdinary.Count != 0 ||
		initial.Current.FundsInCirculation.Sititos != 0 ||
		initial.Current.ReservoirBalance.Sititos != 100000000 ||
		initial.Current.TotalSupply.Sititos != 100000000 {
		t.Fatalf("unexpected empty-ledger analytics: %+v", initial)
	}

	funded := move(t, ctx, transferService, "analytics-fund", "system-reservoir", "alice", 250)
	var funding struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(funded.Body, &funding); err != nil {
		t.Fatal(err)
	}
	var postedAt time.Time
	if err := conn.QueryRow(ctx, "SELECT posted_at FROM ledger_transactions WHERE id = $1", funding.ID).Scan(&postedAt); err != nil {
		t.Fatal(err)
	}
	if postedAt.IsZero() {
		t.Fatal("funding transfer has no posting timestamp")
	}
	inclusive, err := service.Execute(ctx, &analytics.Window{From: postedAt, To: postedAt.Add(time.Microsecond)})
	if err != nil || inclusive.Transfers.Count != 1 || inclusive.Transfers.TotalVolume.Sititos != "250" {
		t.Fatalf("window including start missed transfer: %+v %v", inclusive, err)
	}
	exclusive, err := service.Execute(ctx, &analytics.Window{
		From: postedAt.Add(-time.Microsecond), To: postedAt,
	})
	if err != nil || exclusive.Transfers.Count != 0 || exclusive.Transfers.TotalVolume.Sititos != "0" {
		t.Fatalf("window ending at timestamp included transfer: %+v %v", exclusive, err)
	}

	replay, err := transferService.Execute(ctx, "analytics-fund", transfer.Request{
		SourceID: "system-reservoir", DestinationID: "alice", Sititos: 250,
	})
	if err != nil || !reflect.DeepEqual(replay, funded) {
		t.Fatalf("idempotent replay changed transfer: %+v %v", replay, err)
	}
	move(t, ctx, transferService, "analytics-spend", "alice", "bob", 70)
	move(t, ctx, transferService, "analytics-return", "bob", "alice", 20)
	before := ledgerContents(t, ctx, conn)
	got, err := service.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transfers.Count != 3 || got.Transfers.TotalVolume.Sititos != "340" ||
		got.Transfers.ReservoirDistribution.Count != 1 ||
		got.Transfers.ReservoirDistribution.Volume.Sititos != "250" ||
		got.Transfers.OrdinaryToOrdinary.Count != 2 ||
		got.Transfers.OrdinaryToOrdinary.Volume.Sititos != "90" ||
		got.TimestampCoverage.UntimestampedTransferCount != 0 ||
		got.TimestampCoverage.UntimestampedTransferVolume.Sititos != "0" ||
		got.Current.FundsInCirculation.Sititos != 250 ||
		got.Current.ReservoirBalance.Sititos != 99999750 ||
		got.Current.TotalSupply.Sititos != 100000000 {
		t.Fatalf("unexpected lifetime analytics: %+v", got)
	}
	if !reflect.DeepEqual(before, ledgerContents(t, ctx, conn)) {
		t.Fatal("analytics changed initialized ledger state")
	}
}

func TestAnalyticsExactAggregateIntegration(t *testing.T) {
	conn, ctx, _ := transferDB(t, true)
	_, err := conn.Exec(ctx, `
BEGIN;
INSERT INTO ledger_transactions (id, reference) VALUES
    ('large-transfer-1', 'fixture'), ('large-transfer-2', 'fixture');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
    ('large-transfer-1', 'alice', -5000000000000000000),
    ('large-transfer-1', 'bob', 5000000000000000000),
    ('large-transfer-2', 'alice', -5000000000000000000),
    ('large-transfer-2', 'bob', 5000000000000000000);
UPDATE ledger_transactions SET posted = TRUE
WHERE id IN ('large-transfer-1', 'large-transfer-2');
COMMIT;`)
	if err != nil {
		t.Fatal(err)
	}
	report, err := analytics.New(postgres.NewAPIStore(conn.Config())).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Transfers.Count != 2 || report.Transfers.TotalVolume.Sititos != "10000000000000000000" ||
		report.Transfers.OrdinaryToOrdinary.Volume.Sititos != "10000000000000000000" {
		t.Fatalf("aggregate did not preserve values above int64: %+v", report.Transfers)
	}
}

type analyticsTraceKey struct{}

type analyticsTracer struct {
	t             *testing.T
	ctx           context.Context
	afterSnapshot func(*pgx.Conn)
}

func (tr *analyticsTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, analyticsTraceKey{}, strings.HasPrefix(data.SQL, "-- name: AnalyticsSnapshot "))
}

func (tr *analyticsTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	isSnapshot, _ := ctx.Value(analyticsTraceKey{}).(bool)
	if !isSnapshot || data.Err != nil {
		return
	}
	var isolation, readOnly string
	if err := conn.QueryRow(tr.ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		tr.t.Error(err)
		return
	}
	if err := conn.QueryRow(tr.ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil {
		tr.t.Error(err)
		return
	}
	if isolation != "repeatable read" || readOnly != "on" {
		tr.t.Errorf("not a read-only consistent snapshot: %s %s", isolation, readOnly)
		return
	}
	tr.afterSnapshot(conn)
}

func TestAnalyticsSnapshotIntegration(t *testing.T) {
	conn, ctx, transferService := transferDB(t, true)
	config := conn.Config()
	config.Tracer = &analyticsTracer{
		t: t, ctx: ctx,
		afterSnapshot: func(*pgx.Conn) {
			move(t, ctx, transferService, "analytics-concurrent", "system-reservoir", "alice", 1)
		},
	}
	report, err := analytics.New(postgres.NewAPIStore(config)).Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Transfers.Count != 0 || report.Transfers.TotalVolume.Sititos != "0" ||
		report.Current.FundsInCirculation.Sititos != 0 ||
		report.Current.ReservoirBalance.Sititos != 100000000 {
		t.Fatalf("analytics mixed snapshots during a concurrent transfer: %+v", report)
	}
	latest, err := analytics.New(postgres.NewAPIStore(conn.Config())).Execute(ctx, nil)
	if err != nil || latest.Transfers.Count != 1 || latest.Current.FundsInCirculation.Sititos != 1 ||
		latest.Current.ReservoirBalance.Sititos != 99999999 {
		t.Fatalf("subsequent snapshot missed committed transfer: %+v %v", latest, err)
	}
}

func TestAnalyticsLegacyTimestampIntegration(t *testing.T) {
	base, ctx := testDB(t)
	schema := "analytics_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	config := base.Config()
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
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("remove analytics schema: %v", err)
		}
	})
	apply := func(file string) {
		t.Helper()
		content, err := os.ReadFile("../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(content), "-- +goose Down")
		if _, err := scoped.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	for _, file := range []string{
		"00001_create_accounts.sql",
		"00002_account_names_and_replays.sql",
		"00003_ledger_initialization.sql",
		"00004_transfers.sql",
	} {
		apply(file)
	}
	if _, err := scoped.Exec(ctx, "INSERT INTO accounts (id, name) VALUES ('alice', 'Alice'), ('bob', 'Bob')"); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Exec(ctx, "SELECT initialize_ledger()"); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Exec(ctx, `
INSERT INTO ledger_transactions (id, reference) VALUES ('legacy-transfer', 'fixture');
INSERT INTO entries (transaction_id, account_id, amount) VALUES
    ('legacy-transfer', 'system-reservoir', -15),
    ('legacy-transfer', 'alice', 15);
UPDATE accounts SET balance = balance - 15 WHERE id = 'system-reservoir';
UPDATE accounts SET balance = balance + 15 WHERE id = 'alice';
UPDATE ledger_transactions SET posted = TRUE WHERE id = 'legacy-transfer';
SET CONSTRAINTS ALL IMMEDIATE;`); err != nil {
		t.Fatal(err)
	}
	apply("00005_transaction_posted_at.sql")

	service := analytics.New(postgres.NewAPIStore(config))
	lifetime, err := service.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lifetime.Transfers.Count != 1 || lifetime.Transfers.TotalVolume.Sititos != "15" ||
		lifetime.TimestampCoverage.UntimestampedTransferCount != 1 ||
		lifetime.TimestampCoverage.UntimestampedTransferVolume.Sititos != "15" {
		t.Fatalf("lifetime omitted legacy transfer or coverage: %+v", lifetime)
	}
	window := &analytics.Window{From: time.Now().UTC().Add(-time.Hour), To: time.Now().UTC().Add(time.Hour)}
	bounded, err := service.Execute(ctx, window)
	if err != nil {
		t.Fatal(err)
	}
	if bounded.Transfers.Count != 0 || bounded.Transfers.TotalVolume.Sititos != "0" ||
		bounded.TimestampCoverage.UntimestampedTransferCount != 1 ||
		bounded.TimestampCoverage.UntimestampedTransferVolume.Sititos != "15" ||
		bounded.Current.FundsInCirculation.Sititos != 15 {
		t.Fatalf("bounded analytics mishandled unknown legacy time: %+v", bounded)
	}
}
