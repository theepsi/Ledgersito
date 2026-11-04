package account

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/theepsi/Ledgersito/internal/domain/money"
)

var ErrInvalidID = errors.New("account: ID must be non-empty and contain no whitespace")
var ErrInvalidName = errors.New("account: name must contain 1-100 Unicode code points after trimming, without control characters")

type Role string

const (
	Ordinary  Role = "ordinary"
	Reservoir Role = "reservoir"
	Origin    Role = "origin"
)

type Account struct {
	id      string
	name    string
	balance money.Money
}

func New(id, name string) (Account, error) {
	if err := ValidateID(id); err != nil {
		return Account{}, err
	}

	name, err := NormalizeName(name)
	if err != nil {
		return Account{}, err
	}
	return Account{id: id, name: name, balance: money.New(0)}, nil
}

func ValidateID(id string) error {
	if id == "" || strings.ContainsFunc(id, unicode.IsSpace) {
		return ErrInvalidID
	}
	return nil
}

func NormalizeName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrInvalidName
	}
	return name, nil
}

func (a Account) Name() string {
	return a.name
}

func (a Account) ID() string {
	return a.id
}

func (a Account) Balance() money.Money {
	return a.balance
}

func (a Account) Role() Role {
	return Ordinary
}
