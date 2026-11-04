package transfers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

type generator struct {
	calls   int
	err     error
	invalid bool
}

func (g *generator) Generate() (string, error) {
	g.calls++
	if g.invalid {
		return "", nil
	}
	return "transfer-id", g.err
}

type store struct {
	loadErr, recordErr error
	loads, records     int
}

func (s *store) Load(context.Context, transfer.Request) (transfer.Account, transfer.Account, error) {
	s.loads++
	return transfer.Account{Role: account.Reservoir, Balance: 100},
		transfer.Account{Role: account.Ordinary, Balance: 0}, s.loadErr
}

func (s *store) Record(_ context.Context, id string, req transfer.Request, from, to int64) error {
	s.records++
	if id != "transfer-id" || req.Sititos != 25 || from != 75 || to != 25 {
		return errors.New("incorrect recorded transfer")
	}
	return s.recordErr
}

type storage struct {
	store  *store
	calls  int
	replay bool
	err    error
}

func (s *storage) TransferOnce(_ context.Context, key string, req transfer.Request, execute func(transfers.Store) (transfers.Response, error)) (transfers.Response, error) {
	s.calls++
	if key != "key" || req.SourceID != "from" || req.DestinationID != "to" {
		return transfers.Response{}, errors.New("request identity changed")
	}
	if s.err != nil {
		return transfers.Response{}, s.err
	}
	if s.replay {
		return transfers.Response{Status: 201, Body: []byte("stored response")}, nil
	}
	return execute(s.store)
}

func TestTransferService(t *testing.T) {
	failure := errors.New("injected failure")
	for _, tt := range []struct {
		name                                        string
		storageErr, loadErr, generateErr, recordErr error
		replay                                      bool
		wantLoads, wantIDs, wantRecords             int
	}{
		{name: "success", wantLoads: 1, wantIDs: 1, wantRecords: 1},
		{name: "replay avoids business checks", replay: true},
		{name: "key conflict", storageErr: transfers.ErrKeyReused},
		{name: "not initialized", loadErr: transfers.ErrNotInitialized, wantLoads: 1},
		{name: "missing account", loadErr: transfers.ErrAccountNotFound, wantLoads: 1},
		{name: "generator failure", generateErr: failure, wantLoads: 1, wantIDs: 1},
		{name: "record failure", recordErr: failure, wantLoads: 1, wantIDs: 1, wantRecords: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &store{loadErr: tt.loadErr, recordErr: tt.recordErr}
			db := &storage{store: tx, replay: tt.replay, err: tt.storageErr}
			ids := &generator{err: tt.generateErr}
			got, err := transfers.New(ids, db).Execute(context.Background(), "key", transfer.Request{SourceID: "from", DestinationID: "to", Sititos: 25})
			wantErr := errors.Join(tt.storageErr, tt.loadErr, tt.generateErr, tt.recordErr)
			if (err == nil) != (wantErr == nil) {
				t.Fatalf("got %v, want %v", err, wantErr)
			}
			for _, target := range []error{tt.storageErr, tt.loadErr, tt.generateErr, tt.recordErr} {
				if target != nil && !errors.Is(err, target) {
					t.Fatalf("error identity lost: %v", err)
				}
			}
			if tx.loads != tt.wantLoads || ids.calls != tt.wantIDs || tx.records != tt.wantRecords {
				t.Fatalf("unexpected calls: load=%d generate=%d record=%d", tx.loads, ids.calls, tx.records)
			}
			if err == nil {
				want := `{"id":"transfer-id","source_account_id":"from","destination_account_id":"to","amount":{"sititos":25,"currency":"SITO"}}`
				if tt.replay {
					want = "stored response"
				}
				if got.Status != 201 || string(got.Body) != want {
					t.Fatalf("unexpected response: %+v", got)
				}
			}
		})
	}
}

func TestInvalidTransferDoesNotReachStorage(t *testing.T) {
	db := &storage{}
	_, err := transfers.New(&generator{}, db).Execute(context.Background(), "key", transfer.Request{SourceID: "from", DestinationID: "to"})
	if !errors.Is(err, transfer.ErrInvalidAmount) || db.calls != 0 {
		t.Fatalf("invalid request reached storage: %v, %d", err, db.calls)
	}

}

func TestInvalidGeneratedIDIsInternalFailure(t *testing.T) {
	tx := &store{}
	db := &storage{store: tx}
	_, err := transfers.New(&generator{invalid: true}, db).Execute(context.Background(), "key", transfer.Request{SourceID: "from", DestinationID: "to", Sititos: 25})
	if err == nil || errors.Is(err, account.ErrInvalidID) || tx.records != 0 {
		t.Fatalf("invalid generated ID was persisted or mistaken for invalid input: %v", err)
	}
}
