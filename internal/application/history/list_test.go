package history_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/history"
)

type store struct {
	transactions []history.Transaction
	err          error
}

func (s store) ListTransactions(context.Context) ([]history.Transaction, error) {
	return s.transactions, s.err
}

func TestList(t *testing.T) {
	t.Run("returns persisted history", func(t *testing.T) {
		want := []history.Transaction{{
			ID: "opening", Reference: "initial supply",
			Entries: []history.Entry{{
				ID: 1, AccountID: "system-origin",
				Amount: history.Amount{Sititos: -100000000, Currency: "SITO"},
			}},
		}}
		got, err := history.New(store{transactions: want}).List(context.Background())
		if err != nil || len(got) != 1 || got[0].Entries[0].Amount.Sititos != -100000000 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("normalizes empty history", func(t *testing.T) {
		got, err := history.New(store{}).List(context.Background())
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %#v, %v", got, err)
		}
	})

	t.Run("propagates storage failure", func(t *testing.T) {
		want := errors.New("storage failed")
		_, err := history.New(store{err: want}).List(context.Background())
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want wrapped storage error", err)
		}
	})
}
