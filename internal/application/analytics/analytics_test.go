package analytics_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/theepsi/Ledgersito/internal/application/analytics"
)

type store struct {
	snapshot analytics.Snapshot
	err      error
	window   *analytics.Window
	ctx      context.Context
}

func (s *store) Summarize(ctx context.Context, window *analytics.Window) (analytics.Snapshot, error) {
	s.ctx = ctx
	s.window = window
	return s.snapshot, s.err
}

func TestAnalyticsMapsExactSnapshot(t *testing.T) {
	window := &analytics.Window{
		From: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		To:   time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC),
	}
	s := &store{snapshot: analytics.Snapshot{
		Initialized:                 true,
		TransferCount:               2,
		TotalTransferVolume:         "9223372036854775808",
		ReservoirDistributionCount:  1,
		ReservoirDistributionVolume: "250",
		OrdinaryTransferCount:       1,
		OrdinaryTransferVolume:      "70",
		UntimestampedTransferCount:  1,
		UntimestampedTransferVolume: "10000000000000000000",
		FundsInCirculation:          250,
		ReservoirBalance:            99999750,
		TotalSupply:                 100000000,
	}}

	got, err := analytics.New(s).Execute(context.Background(), window)
	if err != nil {
		t.Fatal(err)
	}
	want := analytics.Report{
		Window: window,
		Transfers: analytics.Transfers{
			Count:       2,
			TotalVolume: analytics.Volume{Sititos: "9223372036854775808", Currency: "SITO"},
			ReservoirDistribution: analytics.TransferCategory{
				Count: 1, Volume: analytics.Volume{Sititos: "250", Currency: "SITO"},
			},
			OrdinaryToOrdinary: analytics.TransferCategory{
				Count: 1, Volume: analytics.Volume{Sititos: "70", Currency: "SITO"},
			},
		},
		TimestampCoverage: analytics.TimestampCoverage{
			UntimestampedTransferCount:  1,
			UntimestampedTransferVolume: analytics.Volume{Sititos: "10000000000000000000", Currency: "SITO"},
		},
		Current: analytics.Current{
			FundsInCirculation: analytics.Balance{Sititos: 250, Currency: "SITO"},
			ReservoirBalance:   analytics.Balance{Sititos: 99999750, Currency: "SITO"},
			TotalSupply:        analytics.Balance{Sititos: 100000000, Currency: "SITO"},
		},
	}
	if !reflect.DeepEqual(got, want) || s.window != window {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAnalyticsRejectsUninitializedLedger(t *testing.T) {
	failure := &store{}
	report, err := analytics.New(failure).Execute(context.Background(), nil)
	if !errors.Is(err, analytics.ErrNotInitialized) || !reflect.DeepEqual(report, analytics.Report{}) {
		t.Fatalf("got %+v, %v", report, err)
	}
}

func TestAnalyticsDiscardsPartialResultsOnFailure(t *testing.T) {
	failure := errors.New("query failed")
	s := &store{
		snapshot: analytics.Snapshot{Initialized: true, TransferCount: 3},
		err:      failure,
	}
	ctx := context.Background()
	report, err := analytics.New(s).Execute(ctx, nil)
	if !errors.Is(err, failure) || !reflect.DeepEqual(report, analytics.Report{}) || s.ctx != ctx {
		t.Fatalf("partial result or lost error/context: %+v, %v", report, err)
	}
}
