package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

type AccountStore struct {
	queries *dbsql.Queries
}

var _ createaccount.Store = (*AccountStore)(nil)

func NewAccountStore(db dbsql.DBTX) *AccountStore {
	return &AccountStore{queries: dbsql.New(db)}
}

func (s *AccountStore) Create(ctx context.Context, a account.Account) error {
	err := s.queries.CreateAccount(ctx, dbsql.CreateAccountParams{
		ID:      a.ID(),
		Balance: a.Balance().Sititos(),
		Name:    a.Name(),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			pgErr.TableName == "accounts" && pgErr.ConstraintName == "accounts_pkey" {
			return fmt.Errorf("postgres create account: %w", createaccount.ErrAlreadyExists)
		}
		return fmt.Errorf("postgres create account: %w", err)
	}
	return nil
}
