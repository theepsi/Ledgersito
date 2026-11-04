package transfer

import (
	"errors"

	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/money"
)

var (
	ErrInvalidAmount     = errors.New("transfer: amount must be positive int64 sititos")
	ErrSameAccount       = errors.New("transfer: source and destination must differ")
	ErrNotTransferable   = errors.New("transfer: account roles do not permit this transfer")
	ErrInsufficientFunds = errors.New("transfer: insufficient source funds")
	ErrBalanceOverflow   = errors.New("transfer: resulting balance overflows int64")
)

type Request struct {
	SourceID      string
	DestinationID string
	Sititos       int64
}

func (r Request) Validate() error {
	if err := account.ValidateID(r.SourceID); err != nil {
		return err
	}
	if err := account.ValidateID(r.DestinationID); err != nil {
		return err
	}
	if r.Sititos <= 0 {
		return ErrInvalidAmount
	}
	if r.SourceID == r.DestinationID {
		return ErrSameAccount
	}
	return nil
}

type Account struct {
	Role    account.Role
	Balance int64
}

func Calculate(source, destination Account, sititos int64) (int64, int64, error) {
	if sititos <= 0 {
		return 0, 0, ErrInvalidAmount
	}
	if (source.Role != account.Ordinary && source.Role != account.Reservoir) ||
		destination.Role != account.Ordinary {
		return 0, 0, ErrNotTransferable
	}
	if source.Balance < sititos {
		return 0, 0, ErrInsufficientFunds
	}
	from, err := money.New(source.Balance).Subtract(money.New(sititos))
	if err != nil {
		return 0, 0, ErrBalanceOverflow
	}
	to, err := money.New(destination.Balance).Add(money.New(sititos))
	if err != nil {
		return 0, 0, ErrBalanceOverflow
	}
	return from.Sititos(), to.Sititos(), nil
}
