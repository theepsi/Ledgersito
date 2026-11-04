package createaccount

import (
	"context"
	"errors"
	"fmt"

	"github.com/theepsi/Ledgersito/internal/domain/account"
)

var ErrAlreadyExists = errors.New("account: ID already exists")

type IDGenerator interface {
	Generate() (string, error)
}

type Store interface {
	Create(context.Context, account.Account) error
}

type UseCase struct {
	generator IDGenerator
	store     Store
}

func New(generator IDGenerator, store Store) UseCase {
	return UseCase{generator: generator, store: store}
}

func (u UseCase) Execute(ctx context.Context, name string) (account.Account, error) {
	name, err := account.NormalizeName(name)
	if err != nil {
		return account.Account{}, err
	}
	id, err := u.generator.Generate()
	if err != nil {
		return account.Account{}, fmt.Errorf("create account: generate ID: %w", err)
	}

	a, err := account.New(id, name)
	if err != nil {
		return account.Account{}, fmt.Errorf("create account: %w", err)
	}

	if err := u.store.Create(ctx, a); err != nil {
		return account.Account{}, fmt.Errorf("create account: save: %w", err)
	}

	return a, nil
}
