package createaccount_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

type generatorFunc func() (string, error)

func (f generatorFunc) Generate() (string, error) {
	return f()
}

type storeFunc func(context.Context, account.Account) error

func (f storeFunc) Create(ctx context.Context, a account.Account) error {
	return f(ctx, a)
}

func TestExecuteCreatesAccounts(t *testing.T) {
	var events []string
	var saved []account.Account
	ids := []string{"account-A", "account-B"}
	generated := 0
	generator := generatorFunc(func() (string, error) {
		if generated >= len(ids) {
			t.Fatal("unexpected extra ID generation")
		}
		id := ids[generated]
		generated++
		events = append(events, "generate "+id)
		return id, nil
	})
	store := storeFunc(func(_ context.Context, a account.Account) error {
		events = append(events, "save "+a.ID())
		saved = append(saved, a)
		return nil
	})
	useCase := createaccount.New(generator, store)

	for _, id := range ids {
		got, err := useCase.Execute(context.Background(), "Savings")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID() != id || got.Balance().Sititos() != 0 {
			t.Fatalf("expected account %q with zero sititos, got %+v", id, got)
		}
		if len(saved) != generated || saved[len(saved)-1] != got {
			t.Fatal("returned account was not saved before success")
		}
	}

	wantEvents := []string{"generate account-A", "save account-A", "generate account-B", "save account-B"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("got events %v, want %v", events, wantEvents)
	}
}

func TestExecuteReportsFailuresWithoutRetry(t *testing.T) {
	generationErr := errors.New("generator unavailable")
	storageErr := errors.New("storage unavailable")
	tests := []struct {
		name          string
		id            string
		generationErr error
		storageErr    error
		wantErr       error
		wantSaves     int
	}{
		{name: "generator failure", generationErr: generationErr, wantErr: generationErr},
		{name: "empty ID", id: "", wantErr: account.ErrInvalidID},
		{name: "whitespace in ID", id: " account-1", wantErr: account.ErrInvalidID},
		{name: "storage failure", id: "account-1", storageErr: storageErr, wantErr: storageErr, wantSaves: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generations, saves := 0, 0
			generator := generatorFunc(func() (string, error) {
				generations++
				return tt.id, tt.generationErr
			})
			store := storeFunc(func(_ context.Context, a account.Account) error {
				saves++
				if a.ID() != tt.id || a.Balance().Sititos() != 0 {
					t.Fatal("storage received the wrong account")
				}
				return tt.storageErr
			})

			_, err := createaccount.New(generator, store).Execute(context.Background(), "Savings")

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got error %v, want identifiable error %v", err, tt.wantErr)
			}
			if generations != 1 || saves != tt.wantSaves {
				t.Errorf("got %d generations and %d saves, want 1 and %d", generations, saves, tt.wantSaves)
			}
		})
	}
}

func TestExecuteReportsConflictWithoutReplacingAccount(t *testing.T) {
	generations, saves := 0, 0
	stored := make(map[string]account.Account)
	generator := generatorFunc(func() (string, error) {
		generations++
		return "account-1", nil
	})
	store := storeFunc(func(_ context.Context, a account.Account) error {
		saves++
		if _, exists := stored[a.ID()]; exists {
			return fmt.Errorf("account %q: %w", a.ID(), createaccount.ErrAlreadyExists)
		}
		stored[a.ID()] = a
		return nil
	})
	useCase := createaccount.New(generator, store)
	first, err := useCase.Execute(context.Background(), "Savings")
	if err != nil {
		t.Fatalf("initial creation failed: %v", err)
	}

	_, err = useCase.Execute(context.Background(), "Savings")

	if !errors.Is(err, createaccount.ErrAlreadyExists) {
		t.Fatalf("got error %v, want identifiable conflict", err)
	}
	if generations != 2 || saves != 2 {
		t.Errorf("got %d generations and %d saves, want 2 each without retries", generations, saves)
	}
	if len(stored) != 1 || stored[first.ID()] != first {
		t.Fatal("conflict changed the existing account")
	}
}
