package money_test

import (
	"math"
	"testing"

	"github.com/theepsi/Ledgersito/internal/domain/money"
)

func TestArithmetic(t *testing.T) {
	tests := []struct {
		name     string
		operate  func(money.Money, money.Money) (money.Money, error)
		left     int64
		right    int64
		want     int64
		overflow bool
	}{
		{name: "add/positive", operate: money.Money.Add, left: 1250, right: 275, want: 1525},
		{name: "add/opposites", operate: money.Money.Add, left: 250, right: -250, want: 0},
		{name: "add/negative", operate: money.Money.Add, left: -250, right: -75, want: -325},
		{name: "add/maximum plus zero", operate: money.Money.Add, left: math.MaxInt64, right: 0, want: math.MaxInt64},
		{name: "add/minimum plus zero", operate: money.Money.Add, left: math.MinInt64, right: 0, want: math.MinInt64},
		{name: "add/opposite limits", operate: money.Money.Add, left: math.MaxInt64, right: math.MinInt64, want: -1},
		{name: "add/upper overflow", operate: money.Money.Add, left: math.MaxInt64, right: 1, overflow: true},
		{name: "add/lower overflow", operate: money.Money.Add, left: math.MinInt64, right: -1, overflow: true},
		{name: "add/reversed upper overflow", operate: money.Money.Add, left: 1, right: math.MaxInt64, overflow: true},
		{name: "add/reversed lower overflow", operate: money.Money.Add, left: -1, right: math.MinInt64, overflow: true},
		{name: "subtract/negative result", operate: money.Money.Subtract, left: 1250, right: 1500, want: -250},
		{name: "subtract/negative operand", operate: money.Money.Subtract, left: 1250, right: -275, want: 1525},
		{name: "subtract/equal amounts", operate: money.Money.Subtract, left: 250, right: 250, want: 0},
		{name: "subtract/maximum minus zero", operate: money.Money.Subtract, left: math.MaxInt64, right: 0, want: math.MaxInt64},
		{name: "subtract/minimum minus zero", operate: money.Money.Subtract, left: math.MinInt64, right: 0, want: math.MinInt64},
		{name: "subtract/equal minimums", operate: money.Money.Subtract, left: math.MinInt64, right: math.MinInt64, want: 0},
		{name: "subtract/negative minus minimum", operate: money.Money.Subtract, left: -1, right: math.MinInt64, want: math.MaxInt64},
		{name: "subtract/upper overflow", operate: money.Money.Subtract, left: math.MaxInt64, right: -1, overflow: true},
		{name: "subtract/lower overflow", operate: money.Money.Subtract, left: math.MinInt64, right: 1, overflow: true},
		{name: "subtract/zero minus minimum", operate: money.Money.Subtract, left: 0, right: math.MinInt64, overflow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := money.New(tt.left)
			right := money.New(tt.right)

			got, err := tt.operate(left, right)

			if left.Sititos() != tt.left || right.Sititos() != tt.right {
				t.Fatal("operation changed an operand")
			}
			if tt.overflow {
				if err == nil {
					t.Fatal("expected an overflow error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Sititos() != tt.want {
				t.Errorf("got %d sititos, want %d", got.Sititos(), tt.want)
			}
		})
	}
}
