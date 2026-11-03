package account

import (
	"errors"
	"strings"
	"unicode"

	"github.com/theepsi/Ledgersito/internal/domain/money"
)

var ErrInvalidID = errors.New("account: ID must be non-empty and contain no whitespace")

type Account struct {
	id      string
	balance money.Money
}

func New(id string) (Account, error) {
	if id == "" || strings.ContainsFunc(id, unicode.IsSpace) {
		return Account{}, ErrInvalidID
	}

	return Account{id: id, balance: money.New(0)}, nil
}

func (a Account) ID() string {
	return a.id
}

func (a Account) Balance() money.Money {
	return a.balance
}
