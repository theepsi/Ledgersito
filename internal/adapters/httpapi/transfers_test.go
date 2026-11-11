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
	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

type transferService struct {
	calls int
	err   error
	req   transfer.Request
}

func (s *transferService) Execute(ctx context.Context, key string, req transfer.Request) (transfers.Response, error) {
	s.calls++
	s.req = req
	if _, ok := ctx.Deadline(); !ok || key == "" {
		return transfers.Response{}, errors.New("missing deadline or key")
	}
	return transfers.Response{Status: 201, Body: []byte(`{"id":"stored-transfer"}`)}, s.err
}

func TestTransferHTTP(t *testing.T) {
	valid := `{"source_account_id":"from","destination_account_id":"to","amount_sititos":25}`
	for _, tt := range []struct {
		name, method, path, body, media, key, code string
		status                                     int
		repeated                                   bool
	}{
		{name: "success", status: 201},
		{name: "charset", media: "application/json; charset=UTF-8", status: 201},
		{name: "maximum amount", body: strings.Replace(valid, ":25", ":9223372036854775807", 1), status: 201},
		{name: "minimum amount", body: strings.Replace(valid, ":25", ":1", 1), status: 201},
		{name: "maximum key", key: strings.Repeat("x", 128), status: 201},
		{name: "unknown", path: "/transfers/", status: 404, code: "not_found"},
		{name: "encoded path", path: "/%74ransfers", status: 404, code: "not_found"},
		{name: "get", method: "GET", status: 405, code: "method_not_allowed"},
		{name: "head", method: "HEAD", status: 405, code: "method_not_allowed"},
		{name: "query", path: "/transfers?a=b", status: 400, code: "invalid_request"},
		{name: "bare query", path: "/transfers?", status: 400, code: "invalid_request"},
		{name: "missing key", key: "-", status: 400, code: "missing_idempotency_key"},
		{name: "invalid key", key: " a", status: 400, code: "invalid_idempotency_key"},
		{name: "comma key", key: "a,b", status: 400, code: "invalid_idempotency_key"},
		{name: "long key", key: strings.Repeat("x", 129), status: 400, code: "invalid_idempotency_key"},
		{name: "duplicate key", repeated: true, status: 400, code: "invalid_idempotency_key"},
		{name: "missing media", media: "-", status: 415, code: "unsupported_media_type"},
		{name: "media parameter", media: "application/json; other=x", status: 415, code: "unsupported_media_type"},
		{name: "array", body: "[]", status: 400, code: "invalid_request"},
		{name: "null", body: "null", status: 400, code: "invalid_request"},
		{name: "empty", body: "{}", status: 400, code: "invalid_request"},
		{name: "missing field", body: `{"source_account_id":"from","amount_sititos":25}`, status: 400, code: "invalid_request"},
		{name: "unknown field", body: strings.Replace(valid, `"amount_sititos"`, `"Amount_sititos"`, 1), status: 400, code: "invalid_request"},
		{name: "duplicate field", body: strings.Replace(valid, `"amount_sititos":25`, `"amount_sititos":25,"amount_sititos":25`, 1), status: 400, code: "invalid_request"},
		{name: "null ID", body: strings.Replace(valid, `"from"`, `null`, 1), status: 400, code: "invalid_request"},
		{name: "number ID", body: strings.Replace(valid, `"from"`, `123`, 1), status: 400, code: "invalid_request"},
		{name: "empty ID", body: strings.Replace(valid, `"from"`, `""`, 1), status: 400, code: "invalid_account_id"},
		{name: "whitespace ID", body: strings.Replace(valid, `"from"`, `"from\u2003"`, 1), status: 400, code: "invalid_account_id"},
		{name: "surrogate ID", body: strings.Replace(valid, `"from"`, `"\ud800"`, 1), status: 400, code: "invalid_request"},
		{name: "UTF8 ID", body: strings.Replace(valid, `"from"`, "\"\xff\"", 1), status: 400, code: "invalid_request"},
		{name: "same account", body: strings.Replace(valid, `"to"`, `"from"`, 1), status: 400, code: "same_account"},
		{name: "trailing", body: valid + `{}`, status: 400, code: "invalid_request"},
		{name: "malformed", body: valid[:len(valid)-1], status: 400, code: "invalid_request"},
		{name: "zero", body: strings.Replace(valid, ":25", ":0", 1), status: 400, code: "invalid_amount"},
		{name: "negative", body: strings.Replace(valid, ":25", ":-1", 1), status: 400, code: "invalid_amount"},
		{name: "overflow", body: strings.Replace(valid, ":25", ":9223372036854775808", 1), status: 400, code: "invalid_amount"},
		{name: "underflow", body: strings.Replace(valid, ":25", ":-9223372036854775809", 1), status: 400, code: "invalid_amount"},
		{name: "fraction", body: strings.Replace(valid, ":25", ":25.0", 1), status: 400, code: "invalid_request"},
		{name: "exponent", body: strings.Replace(valid, ":25", ":2.5e1", 1), status: 400, code: "invalid_request"},
		{name: "quoted", body: strings.Replace(valid, ":25", `:"25"`, 1), status: 400, code: "invalid_request"},
		{name: "null amount", body: strings.Replace(valid, ":25", ":null", 1), status: 400, code: "invalid_request"},
		{name: "body limit", body: valid + strings.Repeat(" ", 4096-len(valid)), status: 201},
		{name: "body over limit", body: valid + strings.Repeat(" ", 4097-len(valid)), status: 413, code: "request_too_large"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &transferService{}
			h := httpapi.NewRouter(&service{}, s, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			method, path, body, media, key := tt.method, tt.path, tt.body, tt.media, tt.key
			if method == "" {
				method = "POST"
			}
			if path == "" {
				path = "/transfers"
			}
			if body == "" {
				body = valid
			}
			if media == "" {
				media = "application/json"
			}
			if key == "" {
				key = "key"
			}
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			if media != "-" {
				r.Header.Set("Content-Type", media)
			}
			if key != "-" {
				r.Header.Set("Idempotency-Key", key)
			}
			if tt.repeated {
				r.Header.Add("Idempotency-Key", key)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status || (tt.code != "" && !strings.Contains(w.Body.String(), `"`+tt.code+`"`)) {
				t.Fatalf("got %d %s, want %d %s", w.Code, w.Body, tt.status, tt.code)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("missing JSON content type")
			}
			if tt.status == 405 && w.Header().Get("Allow") != "POST" {
				t.Fatal("missing Allow")
			}
			if tt.status >= 400 && s.calls != 0 {
				t.Fatal("invalid request reached application")
			}
			if tt.status == 201 && (s.calls != 1 || w.Body.String() != `{"id":"stored-transfer"}`) {
				t.Fatal("response not preserved")
			}
		})
	}
}

func TestTransferHTTPErrors(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{transfers.ErrNotInitialized, 409, "ledger_not_initialized"},
		{transfers.ErrAccountNotFound, 404, "account_not_found"},
		{transfers.ErrKeyReused, 422, "idempotency_key_reused"},
		{transfer.ErrNotTransferable, 422, "account_not_transferable"},
		{transfer.ErrInsufficientFunds, 422, "insufficient_funds"},
		{transfer.ErrBalanceOverflow, 422, "balance_overflow"},
		{context.DeadlineExceeded, 503, "request_timeout"},
		{errors.New("private database detail"), 500, "internal_error"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			s := &transferService{err: tt.err}
			h := httpapi.NewRouter(&service{}, s, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/transfers", strings.NewReader(`{"source_account_id":"from","destination_account_id":"to","amount_sititos":25}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "key")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), `"`+tt.code+`"`) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("unexpected error: %d %s", w.Code, w.Body)
			}
		})
	}
}
