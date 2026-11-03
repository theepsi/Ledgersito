package money

import (
	"errors"
	"math"
)

var ErrOverflow = errors.New("money: amount outside int64 range")

type Money struct {
	sititos int64
}

func New(sititos int64) Money {
	return Money{sititos: sititos}
}

func (m Money) Sititos() int64 {
	return m.sititos
}

func (m Money) Add(other Money) (Money, error) {
	if (other.sititos > 0 && m.sititos > math.MaxInt64-other.sititos) ||
		(other.sititos < 0 && m.sititos < math.MinInt64-other.sititos) {
		return Money{}, ErrOverflow
	}

	return New(m.sititos + other.sititos), nil
}

func (m Money) Subtract(other Money) (Money, error) {
	if (other.sititos > 0 && m.sititos < math.MinInt64+other.sititos) ||
		(other.sititos < 0 && m.sititos > math.MaxInt64+other.sititos) {
		return Money{}, ErrOverflow
	}

	return New(m.sititos - other.sititos), nil
}
