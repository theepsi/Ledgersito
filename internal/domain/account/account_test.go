package account_test

import (
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/domain/account"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "ordinary ID", id: "account-1"},
		{name: "uppercase preserved", id: "account-A"},
		{name: "lowercase preserved", id: "account-a"},
		{name: "not a UUID", id: "1"},
		{name: "Unicode without whitespace", id: "account-\u00e9"},
		{name: "no length limit", id: strings.Repeat("a", 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := account.New(tt.id, "Savings")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID() != tt.id {
				t.Errorf("got ID %q, want %q", got.ID(), tt.id)
			}
			if got.Role() != account.Ordinary {
				t.Fatalf("normal creation must be ordinary, got %q", got.Role())
			}
			if got.Balance().Sititos() != 0 {
				t.Errorf("got opening balance %d, want zero sititos", got.Balance().Sititos())
			}
		})
	}
}

func TestNewRejectsInvalidIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "space only", id: " "},
		{name: "leading space", id: " account-1"},
		{name: "trailing space", id: "account-1 "},
		{name: "embedded space", id: "account 1"},
		{name: "tab", id: "account\t1"},
		{name: "newline", id: "account\n1"},
		{name: "carriage return", id: "account\r1"},
		{name: "non-breaking space", id: "account\u00a01"},
		{name: "em space", id: "account\u20031"},
		{name: "ideographic space", id: "account\u30001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := account.New(tt.id, "Savings"); err == nil {
				t.Fatalf("expected a validation error for ID %q", tt.id)
			}
		})
	}
}

func TestNewDoesNotReserveIDs(t *testing.T) {
	for range 2 {
		got, err := account.New("account-1", "Savings")
		if err != nil {
			t.Fatalf("repeated construction failed: %v", err)
		}
		if got.ID() != "account-1" || got.Balance().Sititos() != 0 {
			t.Fatal("expected the supplied ID and zero opening balance")
		}
	}
}
