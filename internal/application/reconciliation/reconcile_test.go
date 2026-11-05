package reconciliation_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/reconciliation"
)

type store struct {
	report reconciliation.Report
	err    error
	ctx    context.Context
}

func (s *store) Reconcile(ctx context.Context) (reconciliation.Report, error) {
	s.ctx = ctx
	return s.report, s.err
}

func TestReconciliationDiscardsPartialResultsOnFailure(t *testing.T) {
	failure := errors.New("query failed")
	s := &store{
		report: reconciliation.Report{Initialized: true, Findings: []reconciliation.Finding{
			{Code: "account_balance_mismatch", ID: "savings", Message: "Cached balance differs from entries."},
		}},
		err: failure,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	report, err := reconciliation.New(s).Execute(ctx)
	if !errors.Is(err, failure) || !reflect.DeepEqual(report, reconciliation.Report{}) || s.ctx != ctx {
		t.Fatalf("partial result or lost error/context: %+v, %v", report, err)
	}
}
