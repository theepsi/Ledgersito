package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
)

type readinessCheckerFunc func(context.Context) error

func (f readinessCheckerFunc) CheckReadiness(ctx context.Context) error {
	return f(ctx)
}

func readinessRouter(checker httpapi.ReadinessChecker) *httpapi.Handler {
	return httpapi.NewRouter(&service{}, nil, nil, nil, checker, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHealthAndReadinessHTTP(t *testing.T) {
	t.Run("health is independent from readiness", func(t *testing.T) {
		calls := 0
		handler := readinessRouter(readinessCheckerFunc(func(context.Context) error {
			calls++
			return errors.New("unexpected database check")
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/healthz", nil))
		if response.Code != 200 || response.Body.String() != `{"status":"ok"}` || calls != 0 {
			t.Fatalf("got %d %q and %d readiness calls", response.Code, response.Body.String(), calls)
		}
	})

	t.Run("ready when database and schema check succeeds", func(t *testing.T) {
		handler := readinessRouter(readinessCheckerFunc(func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 5*time.Second {
				t.Fatal("readiness check has no five-second deadline")
			}
			return nil
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
		if response.Code != 200 || response.Body.String() != `{"status":"ready"}` {
			t.Fatalf("got %d %q", response.Code, response.Body.String())
		}
	})

	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "database failure", err: errors.New("secret connection details")},
		{name: "timeout", err: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := readinessRouter(readinessCheckerFunc(func(context.Context) error {
				return tt.err
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
			body := response.Body.String()
			if response.Code != 503 || !strings.Contains(body, `"code":"not_ready"`) ||
				strings.Contains(body, "secret") || strings.Contains(body, "request_timeout") {
				t.Fatalf("got %d %q", response.Code, body)
			}
		})
	}

	t.Run("blocked check respects request deadline", func(t *testing.T) {
		handler := readinessRouter(readinessCheckerFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		request := httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx)
		response := httptest.NewRecorder()
		start := time.Now()
		handler.ServeHTTP(response, request)
		if response.Code != 503 || !strings.Contains(response.Body.String(), `"code":"not_ready"`) {
			t.Fatalf("got %d %q", response.Code, response.Body.String())
		}
		if time.Since(start) > time.Second {
			t.Fatal("readiness check exceeded the request deadline")
		}
	})

	t.Run("methods and unknown paths", func(t *testing.T) {
		calls := 0
		handler := readinessRouter(readinessCheckerFunc(func(context.Context) error {
			calls++
			return nil
		}))
		for _, tt := range []struct {
			method, path string
			status       int
			code         string
			allow        string
		}{
			{method: "HEAD", path: "/healthz", status: 405, code: "method_not_allowed", allow: "GET"},
			{method: "POST", path: "/readyz", status: 405, code: "method_not_allowed", allow: "GET"},
			{method: "GET", path: "/healthz?probe=1", status: 400, code: "invalid_request"},
			{method: "GET", path: "/readyz?", status: 400, code: "invalid_request"},
			{method: "GET", path: "/healthz/", status: 404, code: "not_found"},
			{method: "GET", path: "/readyz/", status: 404, code: "not_found"},
			{method: "GET", path: "/%68ealthz", status: 404, code: "not_found"},
			{method: "GET", path: "/%72eadyz", status: 404, code: "not_found"},
		} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
			if response.Code != tt.status || !strings.Contains(response.Body.String(), tt.code) {
				t.Fatalf("%s %s: got %d %q", tt.method, tt.path, response.Code, response.Body.String())
			}
			if tt.allow != "" && response.Header().Get("Allow") != tt.allow {
				t.Fatalf("%s %s: got Allow %q", tt.method, tt.path, response.Header().Get("Allow"))
			}
		}
		if calls != 0 {
			t.Fatalf("invalid requests made %d readiness checks", calls)
		}
	})
}
