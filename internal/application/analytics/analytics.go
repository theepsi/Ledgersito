package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotInitialized = errors.New("ledger is not initialized")

type Window struct {
	From time.Time
	To   time.Time
}

type Volume struct {
	Sititos  string `json:"sititos"`
	Currency string `json:"currency"`
}

type Balance struct {
	Sititos  int64  `json:"sititos"`
	Currency string `json:"currency"`
}

type TransferCategory struct {
	Count  int64  `json:"count"`
	Volume Volume `json:"volume"`
}

type Transfers struct {
	Count                 int64            `json:"count"`
	TotalVolume           Volume           `json:"total_volume"`
	ReservoirDistribution TransferCategory `json:"reservoir_distribution"`
	OrdinaryToOrdinary    TransferCategory `json:"ordinary_to_ordinary"`
}

type TimestampCoverage struct {
	UntimestampedTransferCount  int64  `json:"untimestamped_transfer_count"`
	UntimestampedTransferVolume Volume `json:"untimestamped_transfer_volume"`
}

type Current struct {
	FundsInCirculation Balance `json:"funds_in_circulation"`
	ReservoirBalance   Balance `json:"reservoir_balance"`
	TotalSupply        Balance `json:"total_supply"`
}

type Snapshot struct {
	Initialized                 bool
	TransferCount               int64
	TotalTransferVolume         string
	ReservoirDistributionCount  int64
	ReservoirDistributionVolume string
	OrdinaryTransferCount       int64
	OrdinaryTransferVolume      string
	UntimestampedTransferCount  int64
	UntimestampedTransferVolume string
	FundsInCirculation          int64
	ReservoirBalance            int64
	TotalSupply                 int64
}

type Report struct {
	Window            *Window           `json:"-"`
	Transfers         Transfers         `json:"transfers"`
	TimestampCoverage TimestampCoverage `json:"timestamp_coverage"`
	Current           Current           `json:"current"`
}

type Store interface {
	Summarize(context.Context, *Window) (Snapshot, error)
}

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Execute(ctx context.Context, window *Window) (Report, error) {
	snapshot, err := s.store.Summarize(ctx, window)
	if err != nil {
		return Report{}, fmt.Errorf("summarize ledger analytics: %w", err)
	}
	if !snapshot.Initialized {
		return Report{}, ErrNotInitialized
	}
	return Report{
		Window: window,
		Transfers: Transfers{
			Count:       snapshot.TransferCount,
			TotalVolume: volume(snapshot.TotalTransferVolume),
			ReservoirDistribution: TransferCategory{
				Count: snapshot.ReservoirDistributionCount, Volume: volume(snapshot.ReservoirDistributionVolume),
			},
			OrdinaryToOrdinary: TransferCategory{
				Count: snapshot.OrdinaryTransferCount, Volume: volume(snapshot.OrdinaryTransferVolume),
			},
		},
		TimestampCoverage: TimestampCoverage{
			UntimestampedTransferCount:  snapshot.UntimestampedTransferCount,
			UntimestampedTransferVolume: volume(snapshot.UntimestampedTransferVolume),
		},
		Current: Current{
			FundsInCirculation: Balance{Sititos: snapshot.FundsInCirculation, Currency: "SITO"},
			ReservoirBalance:   Balance{Sititos: snapshot.ReservoirBalance, Currency: "SITO"},
			TotalSupply:        Balance{Sititos: snapshot.TotalSupply, Currency: "SITO"},
		},
	}, nil
}

func volume(sititos string) Volume {
	return Volume{Sititos: sititos, Currency: "SITO"}
}
