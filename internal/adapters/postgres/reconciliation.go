package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres/dbsql"
	"github.com/theepsi/Ledgersito/internal/application/reconciliation"
)

func (s *APIStore) Reconcile(ctx context.Context) (report reconciliation.Report, err error) {
	conn, err := pgx.ConnectConfig(ctx, s.config)
	if err != nil {
		return report, fmt.Errorf("connect for reconciliation: %w", err)
	}
	defer closeConnection(conn, &err)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return report, err
	}
	defer rollbackTransaction(tx, &err)
	q := dbsql.New(tx)
	state, err := q.ReconciliationState(ctx)
	if err != nil {
		return reconciliation.Report{}, err
	}
	report.Initialized = state.Initialized
	if !state.CleanUninitialized {
		rows, err := q.ReconciliationFindings(ctx)
		if err != nil {
			return reconciliation.Report{}, err
		}
		for _, row := range rows {
			report.Findings = append(report.Findings, reconciliation.Finding{
				Code: row.Code, ID: row.SubjectID, Message: row.Message,
			})
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return reconciliation.Report{}, err
	}
	return report, nil
}
