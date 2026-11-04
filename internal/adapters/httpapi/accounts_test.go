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
	"github.com/theepsi/Ledgersito/internal/application/accounts"
)

type service struct {
	calls int
	err   error
}

func (s *service) Create(ctx context.Context, key, name string) (accounts.Response, error) {
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		return accounts.Response{}, errors.New("missing deadline")
	}
	return accounts.Response{Status: 201, Body: []byte(`{"id":"one","name":"Savings","balance":{"sititos":0,"currency":"SITO"}}`)}, s.err
}

func (s *service) List(context.Context) ([]accounts.Account, error) {
	s.calls++
	return []accounts.Account{}, s.err
}

func TestHTTPValidation(t *testing.T) {
	valid := `{"name":"Savings"}`
	for _, tt := range []struct {
		name, method, path, body, media, key, code string
		status                                     int
		repeated                                   bool
	}{
		{name: "valid", method: "POST", path: "/accounts", body: valid, media: "application/json", key: "key", status: 201},
		{name: "charset", method: "POST", path: "/accounts", body: valid, media: "application/json; charset=UTF-8", key: "key", status: 201},
		{name: "empty listing", method: "GET", path: "/accounts", status: 200},
		{name: "unknown path", method: "GET", path: "/accounts/", status: 404, code: "not_found"},
		{name: "method", method: "PUT", path: "/accounts", status: 405, code: "method_not_allowed"},
		{name: "head", method: "HEAD", path: "/accounts", status: 405},
		{name: "query", method: "GET", path: "/accounts?limit=1", status: 400, code: "invalid_request"},
		{name: "bare query", method: "GET", path: "/accounts?", status: 400, code: "invalid_request"},
		{name: "missing key", method: "POST", path: "/accounts", body: valid, media: "application/json", status: 400, code: "missing_idempotency_key"},
		{name: "repeated key", method: "POST", path: "/accounts", body: valid, media: "application/json", key: "key", repeated: true, status: 400, code: "invalid_idempotency_key"},
		{name: "key spaces", method: "POST", path: "/accounts", body: valid, media: "application/json", key: " key", status: 400, code: "invalid_idempotency_key"},
		{name: "long key", method: "POST", path: "/accounts", body: valid, media: "application/json", key: strings.Repeat("x", 129), status: 400, code: "invalid_idempotency_key"},
		{name: "max key", method: "POST", path: "/accounts", body: valid, media: "application/json", key: strings.Repeat("x", 128), status: 201},
		{name: "missing media", method: "POST", path: "/accounts", body: valid, key: "key", status: 415, code: "unsupported_media_type"},
		{name: "media parameter", method: "POST", path: "/accounts", body: valid, media: "application/json; unknown=yes", key: "key", status: 415, code: "unsupported_media_type"},
		{name: "duplicate", method: "POST", path: "/accounts", body: `{"name":"one","name":"two"}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "unknown field", method: "POST", path: "/accounts", body: `{"name":"one","extra":1}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "case sensitive field", method: "POST", path: "/accounts", body: `{"Name":"one"}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "null", method: "POST", path: "/accounts", body: `{"name":null}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "array", method: "POST", path: "/accounts", body: `[]`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "empty object", method: "POST", path: "/accounts", body: `{}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "trailing JSON", method: "POST", path: "/accounts", body: valid + `{}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "malformed", method: "POST", path: "/accounts", body: `{"name":`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "empty name", method: "POST", path: "/accounts", body: `{"name":" "}`, media: "application/json", key: "key", status: 400, code: "invalid_name"},
		{name: "invalid UTF8", method: "POST", path: "/accounts", body: "{\"name\":\"\xff\"}", media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "invalid surrogate", method: "POST", path: "/accounts", body: `{"name":"\ud800"}`, media: "application/json", key: "key", status: 400, code: "invalid_request"},
		{name: "valid surrogate pair", method: "POST", path: "/accounts", body: `{"name":"\ud83d\udc09"}`, media: "application/json", key: "key", status: 201},
		{name: "escaped backslash", method: "POST", path: "/accounts", body: `{"name":"\\ud800"}`, media: "application/json", key: "key", status: 201},
		{name: "comma key", method: "POST", path: "/accounts", body: valid, media: "application/json", key: "a,b", status: 400, code: "invalid_idempotency_key"},
		{name: "at body limit", method: "POST", path: "/accounts", body: valid + strings.Repeat(" ", 4096-len(valid)), media: "application/json", key: "key", status: 201},
		{name: "over body limit", method: "POST", path: "/accounts", body: valid + strings.Repeat(" ", 4097-len(valid)), media: "application/json", key: "key", status: 413, code: "request_too_large"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{}
			handler := httpapi.New(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.media != "" {
				req.Header.Set("Content-Type", tt.media)
			}
			if tt.key != "" {
				req.Header.Set("Idempotency-Key", tt.key)
			}
			if tt.repeated {
				req.Header.Add("Idempotency-Key", tt.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.status || (tt.code != "" && !strings.Contains(w.Body.String(), `"`+tt.code+`"`)) {
				t.Fatalf("got %d %s, want %d %s", w.Code, w.Body.String(), tt.status, tt.code)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("missing JSON content type")
			}
			if tt.status == 405 && w.Header().Get("Allow") != "GET, POST" {
				t.Fatal("missing Allow")
			}
			if tt.status >= 400 && s.calls != 0 {
				t.Fatal("invalid request reached application")
			}
			if tt.name == "empty listing" && w.Body.String() != `{"accounts":[]}` {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestApplicationErrors(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{accounts.ErrKeyReused, 422, "idempotency_key_reused"},
		{context.DeadlineExceeded, 503, "request_timeout"},
		{errors.New("secret SQL details"), 500, "internal_error"},
	} {
		s := &service{err: tt.err}
		handler := httpapi.New(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
		req := httptest.NewRequest("POST", "/accounts", strings.NewReader(`{"name":"Savings"}`))
		req.Header.Set("Idempotency-Key", "key")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.code) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("got %d %s", w.Code, w.Body.String())
		}
	}
}
