package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

func (s *APIStore) TransferOnce(ctx context.Context, key string, req transfer.Request, execute func(transfers.Store) (transfers.Response, error)) (reply transfers.Response, err error) {
	defer s.observeStorage(ctx, "transfer", &err)()
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return reply, fmt.Errorf("connect for transfer: %w", err)
	}
	defer closeConnection(conn, &err)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return reply, err
	}
	defer rollbackTransaction(tx, &err)
	q := dbsql.New(tx)
	inserted, err := q.ReserveTransfer(ctx, dbsql.ReserveTransferParams{
		Key: key, SourceAccountID: req.SourceID, DestinationAccountID: req.DestinationID, Amount: req.Sititos,
	})
	if err != nil {
		return reply, err
	}
	if inserted == 0 {
		stored, err := q.GetTransferReplay(ctx, key)
		if err != nil {
			return reply, err
		}
		if stored.SourceAccountID != req.SourceID || stored.DestinationAccountID != req.DestinationID || stored.Amount != req.Sititos {
			return reply, transfers.ErrKeyReused
		}
		if !stored.StatusCode.Valid || stored.StatusCode.Int32 != 201 || len(stored.ResponseBody) == 0 {
			return reply, errors.New("incomplete committed transfer replay")
		}
		reply = transfers.Response{Status: int(stored.StatusCode.Int32), Body: stored.ResponseBody}
	} else {
		reply, err = execute(&transferStore{q: q})
		if err != nil {
			return transfers.Response{}, err
		}
		if reply.Status != 201 || len(reply.Body) == 0 {
			return transfers.Response{}, errors.New("invalid transfer response")
		}
		if err := q.FinishTransfer(ctx, dbsql.FinishTransferParams{Key: key, ResponseBody: reply.Body}); err != nil {
			return transfers.Response{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return transfers.Response{}, err
	}
	return reply, nil
}

type transferStore struct {
	q *dbsql.Queries
}

func (s *transferStore) Load(ctx context.Context, req transfer.Request) (transfer.Account, transfer.Account, error) {
	// Acquire the write-compatible table lock before row locks so an initializer
	// cannot interleave its stronger table lock between our read and write.
	if err := s.q.LockTransferAccountsTable(ctx); err != nil {
		return transfer.Account{}, transfer.Account{}, err
	}
	initialized, err := s.q.LedgerInitialized(ctx)
	if err != nil {
		return transfer.Account{}, transfer.Account{}, err
	}
	if !initialized {
		return transfer.Account{}, transfer.Account{}, transfers.ErrNotInitialized
	}
	rows, err := s.q.LockTransferAccounts(ctx, dbsql.LockTransferAccountsParams{
		SourceID: req.SourceID, DestinationID: req.DestinationID,
	})
	if err != nil {
		return transfer.Account{}, transfer.Account{}, err
	}
	if len(rows) != 2 {
		return transfer.Account{}, transfer.Account{}, transfers.ErrAccountNotFound
	}
	var source, destination transfer.Account
	for _, row := range rows {
		a := transfer.Account{Role: account.Role(row.Role), Balance: row.Balance}
		if row.ID == req.SourceID {
			source = a
		} else {
			destination = a
		}
	}
	return source, destination, nil
}

func (s *transferStore) Record(ctx context.Context, id string, req transfer.Request, from, to int64) error {
	if err := s.q.CreateTransferTransaction(ctx, id); err != nil {
		return err
	}
	for _, entry := range []dbsql.AddTransferEntryParams{
		{TransactionID: id, AccountID: req.SourceID, Amount: -req.Sititos},
		{TransactionID: id, AccountID: req.DestinationID, Amount: req.Sititos},
	} {
		if err := s.q.AddTransferEntry(ctx, entry); err != nil {
			return err
		}
	}
	for _, balance := range []dbsql.SetTransferBalanceParams{
		{ID: req.SourceID, Balance: from}, {ID: req.DestinationID, Balance: to},
	} {
		count, err := s.q.SetTransferBalance(ctx, balance)
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("locked transfer account disappeared")
		}
	}
	return s.q.PostTransferTransaction(ctx, id)
}
