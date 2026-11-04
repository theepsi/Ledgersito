package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

var ErrKeyReused = errors.New("accounts: idempotency key reused with different input")

type Balance struct {
	Sititos  int64  `json:"sititos"`
	Currency string `json:"currency"`
}

type Account struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Balance Balance `json:"balance"`
}

type Response struct {
	Status int
	Body   []byte
}

type Storage interface {
	// CreateOnce calls create only for a new key, with a transaction-bound store.
	// The account and response commit together; an existing key replays or conflicts.
	CreateOnce(context.Context, string, string, func(createaccount.Store) (Response, error)) (Response, error)
	List(context.Context) ([]Account, error)
}

type Service struct {
	generator createaccount.IDGenerator
	storage   Storage
}

func New(generator createaccount.IDGenerator, storage Storage) *Service {
	return &Service{generator: generator, storage: storage}
}

func (s *Service) Create(ctx context.Context, key, name string) (Response, error) {
	name, err := account.NormalizeName(name)
	if err != nil {
		return Response{}, err
	}
	reply, err := s.storage.CreateOnce(ctx, key, name, func(store createaccount.Store) (Response, error) {
		a, err := createaccount.New(s.generator, store).Execute(ctx, name)
		if err != nil {
			return Response{}, err
		}
		body, err := json.Marshal(Account{
			ID: a.ID(), Name: a.Name(),
			Balance: Balance{Sititos: a.Balance().Sititos(), Currency: "SITO"},
		})
		if err != nil {
			return Response{}, fmt.Errorf("encode created account: %w", err)
		}
		return Response{Status: 201, Body: body}, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("create account operation: %w", err)
	}
	return reply, nil
}

func (s *Service) List(ctx context.Context) ([]Account, error) {
	result, err := s.storage.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	if result == nil {
		result = []Account{}
	}
	return result, nil
}
