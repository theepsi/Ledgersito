package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type store struct {
	created bool
	err     error
}

func (s store) Initialize(context.Context) (bool, error) {
	return s.created, s.err
}

func TestCommandReportsInitializationOutcome(t *testing.T) {
	for _, created := range []bool{true, false} {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		if err := initialize(context.Background(), store{created: created}, logger); err != nil {
			t.Fatal(err)
		}
		want := "already initialized"
		if created {
			want = "ledger initialized"
		}
		if !strings.Contains(output.String(), want) {
			t.Fatal(output.String())
		}
	}
	failure := errors.New("conflicting state")
	if err := initialize(context.Background(), store{err: failure}, slog.Default()); !errors.Is(err, failure) {
		t.Fatal("command lost initialization failure")
	}
}

func TestCommandRequiresExplicitDatabase(t *testing.T) {
	if err := run(context.Background(), "", slog.Default()); err == nil {
		t.Fatal("missing database accepted")
	}
	if err := run(context.Background(), "postgres://user:secret@host:invalid/db", slog.Default()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid configuration accepted or credentials exposed")
	}
}
