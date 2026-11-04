package accounts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
	"github.com/theepsi/Ledgersito/internal/domain/account"
)

type generator struct{ calls int }

func (g *generator) Generate() (string, error) {
	g.calls++
	return "generated-id", nil
}

type storage struct {
	calls int
	name  string
	reply accounts.Response
	err   error
	saved account.Account
}

func (s *storage) Create(_ context.Context, a account.Account) error {
	s.saved = a
	return s.err
}

func (s *storage) CreateOnce(ctx context.Context, key, name string, create func(createaccount.Store) (accounts.Response, error)) (accounts.Response, error) {
	s.calls++
	s.name = name
	if s.err != nil {
		return accounts.Response{}, s.err
	}
	if s.reply.Status != 0 {
		return s.reply, nil
	}
	reply, err := create(s)
	if err == nil {
		s.reply = reply
	}
	return reply, err
}

func (s *storage) List(context.Context) ([]accounts.Account, error) { return nil, s.err }

func TestCreationNormalizesBeforeTransactionAndReplays(t *testing.T) {
	g, s := &generator{}, &storage{}
	service := accounts.New(g, s)
	first, err := service.Create(context.Background(), "key", " Savings ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), "key", "Savings")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != 201 || string(first.Body) != string(second.Body) || g.calls != 1 ||
		s.name != "Savings" || s.saved.Name() != "Savings" {
		t.Fatalf("incorrect creation/replay: %+v %+v", first, second)
	}
}

func TestInvalidNameNeverReservesKey(t *testing.T) {
	g, s := &generator{}, &storage{}
	_, err := accounts.New(g, s).Create(context.Background(), "key", " ")
	if !errors.Is(err, account.ErrInvalidName) || s.calls != 0 || g.calls != 0 {
		t.Fatalf("invalid input reached dependencies: %v", err)
	}
}

func TestStorageFailuresRemainIdentifiable(t *testing.T) {
	failure := errors.New("unavailable")
	service := accounts.New(&generator{}, &storage{err: failure})
	if _, err := service.Create(context.Background(), "key", "Savings"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := service.List(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
