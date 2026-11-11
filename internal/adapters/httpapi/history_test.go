package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/application/history"
)

type transactionHistory struct {
	calls        int
	transactions []history.Transaction
	err          error
}

func (s *transactionHistory) List(ctx context.Context) ([]history.Transaction, error) {
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	return s.transactions, s.err
}

func TestTransactionHistoryHTTP(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, wantBody, code string
		status                             int
	}{
		{name: "history", method: "GET", path: "/transactions", status: 200,
			wantBody: `{"transactions":[{"id":"opening","reference":"initial supply","entries":[{"entry_id":1,"account_id":"system-origin","amount":{"sititos":-100000000,"currency":"SITO"}}]}]}`},
		{name: "empty history", method: "GET", path: "/transactions", status: 200, wantBody: `{"transactions":[]}`},
		{name: "method", method: "HEAD", path: "/transactions", status: 405, code: "method_not_allowed"},
		{name: "other method", method: "POST", path: "/transactions", status: 405, code: "method_not_allowed"},
		{name: "trailing slash", method: "GET", path: "/transactions/", status: 404, code: "not_found"},
		{name: "encoded path", method: "GET", path: "/%74ransactions", status: 404, code: "not_found"},
		{name: "query", method: "GET", path: "/transactions?limit=1", status: 400, code: "invalid_request"},
		{name: "bare query", method: "GET", path: "/transactions?", status: 400, code: "invalid_request"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transactions := []history.Transaction(nil)
			if tt.name == "history" {
				transactions = []history.Transaction{{
					ID: "opening", Reference: "initial supply",
					Entries: []history.Entry{{
						ID: 1, AccountID: "system-origin",
						Amount: history.Amount{Sititos: -100000000, Currency: "SITO"},
					}},
				}}
			}
			txHistory := &transactionHistory{transactions: transactions}
			handler := httpapi.NewRouter(&service{}, nil, txHistory, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.status || (tt.code != "" && !strings.Contains(w.Body.String(), `"`+tt.code+`"`)) {
				t.Fatalf("got %d %s, want %d %s", w.Code, w.Body.String(), tt.status, tt.code)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Fatalf("got %s, want %s", w.Body.String(), tt.wantBody)
			}
			if tt.status == 405 && w.Header().Get("Allow") != "GET" {
				t.Fatalf("got Allow %q", w.Header().Get("Allow"))
			}
			if tt.status >= 400 && txHistory.calls != 0 {
				t.Fatal("invalid request reached history service")
			}
		})
	}
}

func TestTransactionHistoryErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "timeout", err: context.DeadlineExceeded, status: 503, code: "request_timeout"},
		{name: "storage failure", err: errors.New("private database detail"), status: 500, code: "internal_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			txHistory := &transactionHistory{err: tt.err}
			handler := httpapi.NewRouter(&service{}, nil, txHistory, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := httptest.NewRequest("GET", "/transactions", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), `"`+tt.code+`"`) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}
