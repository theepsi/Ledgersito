package transfer_test

import (
	"errors"
	"math"
	"testing"

	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

func TestRequestValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  transfer.Request
		err  error
	}{
		{"valid", transfer.Request{"source", "destination", 1}, nil},
		{"maximum", transfer.Request{"source", "destination", math.MaxInt64}, nil},
		{"empty source", transfer.Request{"", "destination", 1}, account.ErrInvalidID},
		{"destination whitespace", transfer.Request{"source", "dest\u2003ination", 1}, account.ErrInvalidID},
		{"case sensitive IDs", transfer.Request{"source", "Source", 1}, nil},
		{"same account", transfer.Request{"source", "source", 1}, transfer.ErrSameAccount},
		{"zero", transfer.Request{"source", "destination", 0}, transfer.ErrInvalidAmount},
		{"negative", transfer.Request{"source", "destination", -1}, transfer.ErrInvalidAmount},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
		})
	}
}

func TestCalculate(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		source, destination      transfer.Account
		amount, wantFrom, wantTo int64
		err                      error
	}{
		{"distribution", transfer.Account{account.Reservoir, 1000}, transfer.Account{account.Ordinary, 0}, 250, 750, 250, nil},
		{"entire balance", transfer.Account{account.Ordinary, 250}, transfer.Account{account.Ordinary, 10}, 250, 0, 260, nil},
		{"insufficient", transfer.Account{account.Ordinary, 249}, transfer.Account{account.Ordinary, 0}, 250, 0, 0, transfer.ErrInsufficientFunds},
		{"origin source", transfer.Account{account.Origin, -1000}, transfer.Account{account.Ordinary, 0}, 1, 0, 0, transfer.ErrNotTransferable},
		{"origin destination", transfer.Account{account.Ordinary, 100}, transfer.Account{account.Origin, -1000}, 1, 0, 0, transfer.ErrNotTransferable},
		{"reservoir destination", transfer.Account{account.Ordinary, 100}, transfer.Account{account.Reservoir, 1000}, 1, 0, 0, transfer.ErrNotTransferable},
		{"overflow", transfer.Account{account.Ordinary, 100}, transfer.Account{account.Ordinary, math.MaxInt64}, 1, 0, 0, transfer.ErrBalanceOverflow},
		{"max boundary", transfer.Account{account.Ordinary, 1}, transfer.Account{account.Ordinary, math.MaxInt64 - 1}, 1, 0, math.MaxInt64, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			from, to, err := transfer.Calculate(tt.source, tt.destination, tt.amount)
			if !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
			if err == nil && (from != tt.wantFrom || to != tt.wantTo) {
				t.Fatalf("got %d, %d; want %d, %d", from, to, tt.wantFrom, tt.wantTo)
			}
		})
	}
}
