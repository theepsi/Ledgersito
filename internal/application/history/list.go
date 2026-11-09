package history

import (
	"context"
	"fmt"
)

type Amount struct {
	Sititos  int64  `json:"sititos"`
	Currency string `json:"currency"`
}

type Entry struct {
	ID        int64  `json:"entry_id"`
	AccountID string `json:"account_id"`
	Amount    Amount `json:"amount"`
}

type Transaction struct {
	ID        string  `json:"id"`
	Reference string  `json:"reference"`
	Entries   []Entry `json:"entries"`
}

type Store interface {
	ListTransactions(context.Context) ([]Transaction, error)
}

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context) ([]Transaction, error) {
	result, err := s.store.ListTransactions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transaction history: %w", err)
	}
	if result == nil {
		result = []Transaction{}
	}
	return result, nil
}
