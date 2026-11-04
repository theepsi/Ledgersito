package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/initledger"
)

func initialize(ctx context.Context, store initledger.Store, logger *slog.Logger) error {
	created, err := initledger.New(store).Execute(ctx)
	if err != nil {
		return err
	}
	message := "already initialized"
	if created {
		message = "ledger initialized"
	}
	logger.Info(message)
	return nil
}

func run(ctx context.Context, url string, logger *slog.Logger) error {
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	return initialize(ctx, postgres.NewAPIStore(config), logger)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 30*time.Second)
	defer timeoutCancel()
	if err := run(ctx, os.Getenv("DATABASE_URL"), logger); err != nil {
		logger.Error("ledger initialization failed; commit outcome may be unknown, rerun safely with the same database", "error", err)
		os.Exit(1)
	}
}
