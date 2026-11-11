//go:build integration && initializationcheck

package postgres_test

import (
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
)

func TestReadinessRequiresCurrentMigrationIntegration(t *testing.T) {
	conn, ctx := initializationDB(t, false)
	store := postgres.NewAPIStore(conn.Config())

	if err := store.CheckReadiness(ctx); err == nil {
		t.Fatal("readiness succeeded without Goose migration state")
	}
	if _, err := conn.Exec(ctx, `
CREATE TABLE goose_db_version (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (5, true)"); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReadiness(ctx); err != nil {
		t.Fatalf("readiness rejected migrated but uninitialized ledger: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (5, false)"); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReadiness(ctx); err == nil {
		t.Fatal("readiness succeeded after the required migration was rolled back")
	}
}
