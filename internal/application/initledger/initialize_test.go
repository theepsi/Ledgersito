package initledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/initledger"
)

type store struct {
	calls   int
	created bool
	err     error
}

func (s *store) Initialize(context.Context) (bool, error) {
	s.calls++
	return s.created, s.err
}

func TestInitialization(t *testing.T) {
	failure := errors.New("inconsistent ledger")
	for _, tt := range []struct {
		name    string
		created bool
		err     error
	}{
		{"new ledger", true, nil},
		{"already initialized", false, nil},
		{"failure", false, failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &store{created: tt.created, err: tt.err}
			created, err := initledger.New(s).Execute(context.Background())
			if created != tt.created || !errors.Is(err, tt.err) || s.calls != 1 {
				t.Fatalf("got %t, %v, %d calls", created, err, s.calls)
			}
		})
	}
}
