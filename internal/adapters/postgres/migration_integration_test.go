//go:build integration && migrationcheck

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestNameMigration(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty"
		if populated {
			name = "populated"
		}
		t.Run(name, func(t *testing.T) {
			conn, ctx := testDB(t)
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Error(err)
				}
			}()
			schema := "migration_" + strings.ToLower(rand.Text())
			if _, err := tx.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			apply := func(file string) {
				t.Helper()
				sql, err := os.ReadFile("../../../migrations/" + file)
				if err != nil {
					t.Fatal(err)
				}
				up, _, _ := strings.Cut(string(sql), "-- +goose Down")
				if _, err := tx.Exec(ctx, up); err != nil {
					t.Fatal(err)
				}
			}
			apply("00001_create_accounts.sql")
			if populated {
				if _, err := tx.Exec(ctx, "INSERT INTO accounts (id, balance) VALUES ('old-a', 42), ('old-b', -7)"); err != nil {
					t.Fatal(err)
				}
			}
			apply("00002_account_names_and_replays.sql")
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if populated {
				want = 2
			}
			if count != want {
				t.Fatalf("migration changed row count: %d", count)
			}
			if populated {
				for id, balance := range map[string]int64{"old-a": 42, "old-b": -7} {
					var gotName string
					var gotBalance int64
					if err := tx.QueryRow(ctx, "SELECT name, balance FROM accounts WHERE id = $1", id).Scan(&gotName, &gotBalance); err != nil {
						t.Fatal(err)
					}
					if gotName != "Unnamed account" || gotBalance != balance {
						t.Fatal("migration changed existing identity/balance or missed backfill")
					}
				}
			}
			var hasDefault bool
			if err := tx.QueryRow(ctx, "SELECT column_default IS NOT NULL FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'accounts' AND column_name = 'name'", schema).Scan(&hasDefault); err != nil {
				t.Fatal(err)
			}
			if hasDefault {
				t.Fatal("new accounts must supply an explicit name")
			}
		})
	}
}
