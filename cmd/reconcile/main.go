package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/reconciliation"
)

func reconcile(ctx context.Context, store reconciliation.Store, stdout, stderr io.Writer) int {
	report, err := reconciliation.New(store).Execute(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Reconciliation failed; no consistency result is available.")
		return 2
	}
	var output bytes.Buffer
	code := 0
	switch {
	case len(report.Findings) > 0:
		for _, f := range report.Findings {
			fmt.Fprint(&output, f.Code)
			if f.ID != "" {
				fmt.Fprintf(&output, " id=%q", f.ID)
			}
			fmt.Fprintf(&output, ": %s\n", f.Message)
		}
		fmt.Fprintf(&output, "%d invariant finding(s).\n", len(report.Findings))
		code = 1
	case !report.Initialized:
		fmt.Fprintln(&output, "Ledger is not initialized.")
		code = 3
	default:
		fmt.Fprintln(&output, "Ledger is consistent.")
	}
	if _, err := output.WriteTo(stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "Could not write reconciliation report.")
		return 2
	}
	return code
}

func run(ctx context.Context, url string, stdout, stderr io.Writer) int {
	if url == "" {
		_, _ = fmt.Fprintln(stderr, "DATABASE_URL is required.")
		return 2
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Invalid DATABASE_URL.")
		return 2
	}
	return reconcile(ctx, postgres.NewAPIStore(config), stdout, stderr)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 30*time.Second)
	defer timeoutCancel()
	os.Exit(run(ctx, os.Getenv("DATABASE_URL"), os.Stdout, os.Stderr))
}
