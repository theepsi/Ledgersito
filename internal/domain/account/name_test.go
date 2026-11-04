package account_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/domain/account"
)

func TestNames(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
		valid bool
	}{
		{" Savings ", "Savings", true},
		{"\u00a0Caf\u00e9\n", "Caf\u00e9", true},
		{"Game  rewards", "Game  rewards", true},
		{strings.Repeat("\u00e9", 100), strings.Repeat("\u00e9", 100), true},
		{"", "", false},
		{" \t\n", "", false},
		{strings.Repeat("\u00e9", 101), "", false},
		{"Game\nrewards", "", false},
		{"Game\trewards", "", false},
		{"Game\x00rewards", "", false},
		{"\xff", "", false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := account.New("account-1", tt.input)
			if !tt.valid {
				if !errors.Is(err, account.ErrInvalidName) {
					t.Fatalf("want invalid name, got %v", err)
				}
				return
			}
			if err != nil || got.Name() != tt.want || got.Balance().Sititos() != 0 {
				t.Fatalf("got %+v, %v; want name %q and zero balance", got, err, tt.want)
			}
		})
	}
}
