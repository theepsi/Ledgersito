package initledger

import (
	"context"
	"fmt"
)

type Store interface {
	Initialize(context.Context) (bool, error)
}

type UseCase struct {
	store Store
}

func New(store Store) UseCase {
	return UseCase{store: store}
}

func (u UseCase) Execute(ctx context.Context) (bool, error) {
	created, err := u.store.Initialize(ctx)
	if err != nil {
		return false, fmt.Errorf("initialize ledger: %w", err)
	}
	return created, nil
}
