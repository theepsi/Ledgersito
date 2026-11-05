package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/reconciliation"
)

type store struct {
	report reconciliation.Report
	err    error
}

func (s store) Reconcile(context.Context) (reconciliation.Report, error) {
	return s.report, s.err
}

func TestReconcileCommandOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		store  store
		code   int
		stdout string
		stderr string
	}{
		{
			name:  "healthy",
			store: store{report: reconciliation.Report{Initialized: true}},
			code:  0, stdout: "Ledger is consistent.\n",
		},
		{
			name: "clean uninitialized", code: 3, stdout: "Ledger is not initialized.\n",
		},
		{
			name: "findings", code: 1,
			store: store{report: reconciliation.Report{Initialized: true, Findings: []reconciliation.Finding{
				{Code: "account_balance_mismatch", ID: "savings\ninjected", Message: "Cached balance differs from entries."},
				{Code: "total_balance_mismatch", Message: "Account balances do not total zero."},
			}}},
			stdout: "account_balance_mismatch id=\"savings\\ninjected\": Cached balance differs from entries.\n" +
				"total_balance_mismatch: Account balances do not total zero.\n2 invariant finding(s).\n",
		},
		{
			name: "partial initialization", code: 1,
			store: store{report: reconciliation.Report{Findings: []reconciliation.Finding{
				{Code: "initialization_record", Message: "Initialization metadata is missing or invalid."},
			}}},
			stdout: "initialization_record: Initialization metadata is missing or invalid.\n1 invariant finding(s).\n",
		},
		{
			name: "operational error suppresses partial report", code: 2,
			store:  store{report: reconciliation.Report{Initialized: true}, err: errors.New("postgres://secret/raw SQL")},
			stderr: "Reconciliation failed; no consistency result is available.\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := reconcile(context.Background(), tt.store, &stdout, &stderr)
			if code != tt.code || stdout.String() != tt.stdout || stderr.String() != tt.stderr {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReconcileCommandRejectsConfigurationWithoutExposingIt(t *testing.T) {
	for _, url := range []string{"", "postgres://user:secret@localhost/%zz"} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), url, &stdout, &stderr); code != 2 || stdout.Len() != 0 ||
			stderr.Len() == 0 || strings.Contains(stderr.String(), "secret") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestReconcileCommandReportsOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	code := reconcile(context.Background(), store{report: reconciliation.Report{Initialized: true}}, failingWriter{}, &stderr)
	if code != 2 || stderr.Len() == 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
