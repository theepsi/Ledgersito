package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/analytics"
)

func (s *APIStore) Summarize(ctx context.Context, window *analytics.Window) (snapshot analytics.Snapshot, err error) {
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return snapshot, fmt.Errorf("connect for ledger analytics: %w", err)
	}
	defer closeConnection(conn, &err)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return snapshot, err
	}
	defer rollbackTransaction(tx, &err)

	params := dbsql.AnalyticsSnapshotParams{}
	if window != nil {
		params.Windowed = true
		params.WindowFrom = pgtype.Timestamptz{Time: window.From, Valid: true}
		params.WindowTo = pgtype.Timestamptz{Time: window.To, Valid: true}
	}
	row, err := dbsql.New(tx).AnalyticsSnapshot(ctx, params)
	if err != nil {
		return analytics.Snapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return analytics.Snapshot{}, err
	}
	return analytics.Snapshot{
		Initialized:                 row.Initialized,
		TransferCount:               row.TransferCount,
		TotalTransferVolume:         row.TotalTransferVolume,
		ReservoirDistributionCount:  row.ReservoirDistributionCount,
		ReservoirDistributionVolume: row.ReservoirDistributionVolume,
		OrdinaryTransferCount:       row.OrdinaryTransferCount,
		OrdinaryTransferVolume:      row.OrdinaryTransferVolume,
		UntimestampedTransferCount:  row.UntimestampedTransferCount,
		UntimestampedTransferVolume: row.UntimestampedTransferVolume,
		FundsInCirculation:          row.FundsInCirculation,
		ReservoirBalance:            row.ReservoirBalance,
		TotalSupply:                 row.TotalSupply,
	}, nil
}
