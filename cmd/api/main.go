package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/transfers"
)

type config struct {
	address  string
	database *pgx.ConnConfig
}

func loadConfig() (config, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}
	database, err := pgx.ParseConfig(url)
	if err != nil {
		return config{}, errors.New("invalid DATABASE_URL")
	}
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	return config{address: address, database: database}, nil
}

type randomIDs struct{}

func (randomIDs) Generate() (string, error) {
	return rand.Text(), nil
}

func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := pgx.ConnectConfig(startup, cfg.database)
	if err == nil {
		err = conn.Ping(startup)
		cleanup, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = errors.Join(err, conn.Close(cleanup))
		closeCancel()
	}
	cancel()
	if err != nil {
		return fmt.Errorf("database startup check failed: %w", err)
	}
	store := postgres.NewAPIStore(cfg.database)
	service := accounts.New(randomIDs{}, store)
	transferService := transfers.New(randomIDs{}, store)
	server := &http.Server{
		Addr: cfg.address, Handler: httpapi.NewRouter(service, transferService, logger),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("API listening", "address", listener.Addr().String())
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return errors.Join(fmt.Errorf("graceful shutdown: %w", err), server.Close())
		}
		if err := <-stopped; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		logger.Info("API stopped")
		return nil
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := loadConfig()
	if err == nil {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err = run(ctx, cfg, logger)
		cancel()
	}
	if err != nil {
		logger.Error("API failed", "error", err)
		os.Exit(1)
	}
}
