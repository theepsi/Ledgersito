package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/history"
)

func (s *APIStore) ListTransactions(ctx context.Context) (result []history.Transaction, err error) {
	defer s.observeStorage(ctx, "list_transactions", &err)()
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return nil, fmt.Errorf("connect for transaction history: %w", err)
	}
	defer closeConnection(conn, &err)
	rows, err := dbsql.New(conn).ListTransactions(ctx)
	if err != nil {
		return nil, err
	}
	result = make([]history.Transaction, 0)
	for _, row := range rows {
		if len(result) == 0 || result[len(result)-1].ID != row.TransactionID {
			result = append(result, history.Transaction{
				ID: row.TransactionID, Reference: row.Reference, Entries: []history.Entry{},
			})
		}
		transaction := &result[len(result)-1]
		transaction.Entries = append(transaction.Entries, history.Entry{
			ID: row.EntryID, AccountID: row.AccountID,
			Amount: history.Amount{Sititos: row.Amount, Currency: "SITO"},
		})
	}
	return result, nil
}
