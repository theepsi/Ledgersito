package reconciliation

import (
	"context"
	"fmt"
)

type Finding struct {
	Code    string
	ID      string
	Message string
}

type Report struct {
	Initialized bool
	Findings    []Finding
}

type Store interface {
	Reconcile(context.Context) (Report, error)
}

type UseCase struct {
	store Store
}

func New(store Store) UseCase {
	return UseCase{store: store}
}

func (u UseCase) Execute(ctx context.Context) (Report, error) {
	report, err := u.store.Reconcile(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reconcile ledger: %w", err)
	}
	return report, nil
}
