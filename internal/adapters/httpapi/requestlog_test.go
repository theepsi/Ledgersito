package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
)

func TestHTTPRequestCompletionLogs(t *testing.T) {
	tests := []struct {
		name, method, path, body, key, route, level string
		status                                      int
	}{
		{name: "account listing", method: "GET", path: "/accounts", route: "/accounts", status: 200, level: "INFO"},
		{name: "transfer route", method: "GET", path: "/transfers", route: "/transfers", status: 405, level: "WARN"},
		{name: "transaction history", method: "GET", path: "/transactions", route: "/transactions", status: 200, level: "INFO"},
		{name: "analytics", method: "GET", path: "/analytics", route: "/analytics", status: 200, level: "INFO"},
		{name: "health", method: "GET", path: "/healthz", route: "/healthz", status: 200, level: "INFO"},
		{name: "readiness", method: "GET", path: "/readyz", route: "/readyz", status: 200, level: "INFO"},
		{name: "query rejected", method: "GET", path: "/healthz?token=private-query", route: "/healthz", status: 400, level: "WARN"},
		{name: "unknown path", method: "GET", path: "/private/path-id", route: "unmatched", status: 404, level: "WARN"},
		{name: "encoded path", method: "GET", path: "/%61nalytics", route: "unmatched", status: 404, level: "WARN"},
		{name: "account creation", method: "POST", path: "/accounts", body: `{"name":"Private account label"}`, key: "private-idempotency-key", route: "/accounts", status: 201, level: "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			checker := readinessCheckerFunc(func(context.Context) error { return nil })
			handler := httpapi.NewRouter(
				&service{},
				&transferService{},
				&transactionHistory{},
				&analyticsService{},
				checker,
				logger,
			)
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if tt.key != "" {
				request.Header.Set("Idempotency-Key", tt.key)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("got status %d, want %d", response.Code, tt.status)
			}

			records := decodeLogRecords(t, output.String())
			completions := completionLogRecords(records)
			if len(completions) != 1 {
				t.Fatalf("got %d completion events in %q", len(completions), output.String())
			}
			record := completions[0]
			if record["event"] != "http_request_completed" ||
				record["method"] != tt.method || record["route"] != tt.route ||
				record["status"] != float64(tt.status) || record["level"] != tt.level {
				t.Fatalf("unexpected completion event: %#v", record)
			}
			duration, ok := record["duration_ms"].(float64)
			if !ok || duration < 0 {
				t.Fatalf("invalid duration_ms: %#v", record["duration_ms"])
			}
			if len(record) != 8 {
				t.Fatalf("unexpected request event fields: %#v", record)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			for _, sensitive := range []string{
				"private-query", "path-id", "Private account label",
				"private-idempotency-key", tt.body,
			} {
				if sensitive != "" && strings.Contains(string(encoded), sensitive) {
					t.Fatalf("request event leaked %q: %s", sensitive, encoded)
				}
			}
		})
	}
}

func TestHTTPRequestCompletionLogsServerError(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	checker := readinessCheckerFunc(func(context.Context) error {
		return context.DeadlineExceeded
	})
	handler := httpapi.NewRouter(&service{}, nil, nil, nil, checker, logger)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 503 {
		t.Fatalf("got status %d, want 503", response.Code)
	}
	completions := completionLogRecords(decodeLogRecords(t, output.String()))
	if len(completions) != 1 {
		t.Fatalf("got %d completion events in %q", len(completions), output.String())
	}
	record := completions[0]
	if record["level"] != "ERROR" || record["status"] != float64(503) || record["route"] != "/readyz" {
		t.Fatalf("unexpected server-error completion event: %#v", record)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "context deadline exceeded") {
		t.Fatalf("request completion event copied the error: %s", encoded)
	}
}

func decodeLogRecords(t *testing.T, output string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid JSON log record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func completionLogRecords(records []map[string]any) []map[string]any {
	var completions []map[string]any
	for _, record := range records {
		if record["event"] == "http_request_completed" {
			completions = append(completions, record)
		}
	}
	return completions
}
