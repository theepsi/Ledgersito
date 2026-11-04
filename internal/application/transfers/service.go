package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

var (
	ErrKeyReused       = errors.New("transfers: idempotency key reused with different input")
	ErrNotInitialized  = errors.New("transfers: ledger not initialized")
	ErrAccountNotFound = errors.New("transfers: account not found")
)

type Response struct {
	Status int
	Body   []byte
}

type IDGenerator interface {
	Generate() (string, error)
}

type Store interface {
	// Load requires initialization and locks both accounts in a consistent order.
	Load(context.Context, transfer.Request) (transfer.Account, transfer.Account, error)
	Record(context.Context, string, transfer.Request, int64, int64) error
}

type Storage interface {
	// TransferOnce replays or conflicts before executing any new business checks.
	// The callback receives a transaction-bound store; writes and response commit together.
	TransferOnce(context.Context, string, transfer.Request, func(Store) (Response, error)) (Response, error)
}

type Service struct {
	generator IDGenerator
	storage   Storage
}

func New(generator IDGenerator, storage Storage) *Service {
	return &Service{generator: generator, storage: storage}
}

func (s *Service) Execute(ctx context.Context, key string, req transfer.Request) (Response, error) {
	if err := req.Validate(); err != nil {
		return Response{}, err
	}
	reply, err := s.storage.TransferOnce(ctx, key, req, func(store Store) (Response, error) {
		source, destination, err := store.Load(ctx, req)
		if err != nil {
			return Response{}, err
		}
		from, to, err := transfer.Calculate(source, destination, req.Sititos)
		if err != nil {
			return Response{}, err
		}
		id, err := s.generator.Generate()
		if err != nil {
			return Response{}, fmt.Errorf("generate transfer ID: %w", err)
		}
		if err := account.ValidateID(id); err != nil {
			return Response{}, fmt.Errorf("invalid generated transfer ID: %v", err)
		}
		if err := store.Record(ctx, id, req, from, to); err != nil {
			return Response{}, err
		}
		body, err := json.Marshal(struct {
			ID            string `json:"id"`
			SourceID      string `json:"source_account_id"`
			DestinationID string `json:"destination_account_id"`
			Amount        struct {
				Sititos  int64  `json:"sititos"`
				Currency string `json:"currency"`
			} `json:"amount"`
		}{
			ID: id, SourceID: req.SourceID, DestinationID: req.DestinationID,
			Amount: struct {
				Sititos  int64  `json:"sititos"`
				Currency string `json:"currency"`
			}{req.Sititos, "SITO"},
		})
		if err != nil {
			return Response{}, fmt.Errorf("encode transfer: %w", err)
		}
		return Response{Status: 201, Body: body}, nil
	})
	if err != nil {
		return Response{}, fmt.Errorf("transfer operation: %w", err)
	}
	return reply, nil
}
