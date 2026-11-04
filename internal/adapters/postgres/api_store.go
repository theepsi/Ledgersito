package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
)

type APIStore struct {
	config *pgx.ConnConfig
}

func NewAPIStore(config *pgx.ConnConfig) *APIStore {
	return &APIStore{config: config.Copy()}
}

func closeConnection(conn *pgx.Conn, result *error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Close(ctx); err != nil {
		*result = errors.Join(*result, fmt.Errorf("close database connection: %w", err))
	}
}

func (s *APIStore) CreateOnce(ctx context.Context, key, name string, create func(createaccount.Store) (accounts.Response, error)) (reply accounts.Response, err error) {
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return reply, fmt.Errorf("connect for account creation: %w", err)
	}
	defer closeConnection(conn, &err)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return reply, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback account creation: %w", rollbackErr))
		}
	}()
	q := dbsql.New(tx)
	// A conflicting insert waits for the owner transaction. At READ COMMITTED,
	// the subsequent SELECT sees its committed result, or we acquire a rolled-back key.
	inserted, err := q.ReserveCreation(ctx, dbsql.ReserveCreationParams{Key: key, RequestName: name})
	if err != nil {
		return reply, err
	}
	if inserted == 0 {
		stored, err := q.GetCreation(ctx, key)
		if err != nil {
			return reply, err
		}
		if stored.RequestName != name {
			return reply, accounts.ErrKeyReused
		}
		if !stored.StatusCode.Valid || stored.StatusCode.Int32 != 201 || stored.ResponseBody == nil {
			return reply, errors.New("incomplete committed account replay")
		}
		reply = accounts.Response{Status: int(stored.StatusCode.Int32), Body: stored.ResponseBody}
	} else {
		reply, err = create(NewAccountStore(tx))
		if err != nil {
			return accounts.Response{}, err
		}
		if reply.Status != 201 || len(reply.Body) == 0 {
			return accounts.Response{}, errors.New("invalid account creation response")
		}
		if err := q.FinishCreation(ctx, dbsql.FinishCreationParams{
			Key: key, StatusCode: pgtype.Int4{Int32: 201, Valid: true}, ResponseBody: reply.Body,
		}); err != nil {
			return accounts.Response{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return accounts.Response{}, err
	}
	return reply, nil
}

func (s *APIStore) List(ctx context.Context) (result []accounts.Account, err error) {
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return nil, err
	}
	defer closeConnection(conn, &err)
	rows, err := dbsql.New(conn).ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result = make([]accounts.Account, 0, len(rows))
	for _, row := range rows {
		result = append(result, accounts.Account{
			ID: row.ID, Name: row.Name,
			Balance: accounts.Balance{Sititos: row.Balance, Currency: "SITO"},
		})
	}
	return result, nil
}
