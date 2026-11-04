package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

type failingDB struct {
	dbsql.DBTX
	err error
}

func (db failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, db.err
}

func TestCreateClassifiesDatabaseErrors(t *testing.T) {
	unavailable := errors.New("connection unavailable")
	tests := []struct {
		name     string
		err      error
		conflict bool
	}{
		{name: "account primary key", err: &pgconn.PgError{Code: "23505", TableName: "accounts", ConstraintName: "accounts_pkey"}, conflict: true},
		{name: "other unique constraint", err: &pgconn.PgError{Code: "23505", TableName: "accounts", ConstraintName: "another_key"}},
		{name: "other table", err: &pgconn.PgError{Code: "23505", TableName: "other", ConstraintName: "accounts_pkey"}},
		{name: "other database error", err: &pgconn.PgError{Code: "42501"}},
		{name: "connection failure", err: unavailable},
	}
	a, err := account.New("account-1", "Savings")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := postgres.NewAccountStore(failingDB{err: tt.err}).Create(context.Background(), a)
			if errors.Is(err, createaccount.ErrAlreadyExists) != tt.conflict {
				t.Fatalf("wrong conflict classification: %v", err)
			}
			if !tt.conflict && !errors.Is(err, tt.err) {
				t.Fatalf("lost underlying error: %v", err)
			}
		})
	}
}
