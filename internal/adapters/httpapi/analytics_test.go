package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/application/analytics"
)

type analyticsService struct {
	calls  int
	window *analytics.Window
	report analytics.Report
	err    error
}

func (s *analyticsService) Execute(ctx context.Context, window *analytics.Window) (analytics.Report, error) {
	s.calls++
	s.window = window
	if _, ok := ctx.Deadline(); !ok {
		return analytics.Report{}, errors.New("missing deadline")
	}
	s.report.Window = window
	return s.report, s.err
}

func testRouter(analyticsSvc *analyticsService) *httpapi.Handler {
	return httpapi.NewRouter(&service{}, nil, nil, analyticsSvc, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAnalyticsHTTP(t *testing.T) {
	report := analytics.Report{
		Transfers: analytics.Transfers{
			Count:       2,
			TotalVolume: analytics.Volume{Sititos: "320", Currency: "SITO"},
			ReservoirDistribution: analytics.TransferCategory{
				Count: 1, Volume: analytics.Volume{Sititos: "250", Currency: "SITO"},
			},
			OrdinaryToOrdinary: analytics.TransferCategory{
				Count: 1, Volume: analytics.Volume{Sititos: "70", Currency: "SITO"},
			},
		},
		TimestampCoverage: analytics.TimestampCoverage{
			UntimestampedTransferCount: 0,
			UntimestampedTransferVolume: analytics.Volume{
				Sititos: "0", Currency: "SITO",
			},
		},
		Current: analytics.Current{
			FundsInCirculation: analytics.Balance{Sititos: 250, Currency: "SITO"},
			ReservoirBalance:   analytics.Balance{Sititos: 99999750, Currency: "SITO"},
			TotalSupply:        analytics.Balance{Sititos: 100000000, Currency: "SITO"},
		},
	}
	for _, tt := range []struct {
		name, method, path, wantBody, code string
		status                             int
	}{
		{
			name: "lifetime", method: "GET", path: "/analytics", status: 200,
			wantBody: `{"period":"lifetime","transfers":{"count":2,"total_volume":{"sititos":"320","currency":"SITO"},"reservoir_distribution":{"count":1,"volume":{"sititos":"250","currency":"SITO"}},"ordinary_to_ordinary":{"count":1,"volume":{"sititos":"70","currency":"SITO"}}},"timestamp_coverage":{"untimestamped_transfer_count":0,"untimestamped_transfer_volume":{"sititos":"0","currency":"SITO"}},"current":{"funds_in_circulation":{"sititos":250,"currency":"SITO"},"reservoir_balance":{"sititos":99999750,"currency":"SITO"},"total_supply":{"sititos":100000000,"currency":"SITO"}}}`,
		},
		{
			name: "window", method: "GET",
			path:     "/analytics?from=2026-01-02T03:04:05%2B02:00&to=2026-01-03T03:04:05%2B02:00",
			status:   200,
			wantBody: `{"period":{"from":"2026-01-02T01:04:05Z","to":"2026-01-03T01:04:05Z"},"transfers":{"count":2,"total_volume":{"sititos":"320","currency":"SITO"},"reservoir_distribution":{"count":1,"volume":{"sititos":"250","currency":"SITO"}},"ordinary_to_ordinary":{"count":1,"volume":{"sititos":"70","currency":"SITO"}}},"timestamp_coverage":{"untimestamped_transfer_count":0,"untimestamped_transfer_volume":{"sititos":"0","currency":"SITO"}},"current":{"funds_in_circulation":{"sititos":250,"currency":"SITO"},"reservoir_balance":{"sititos":99999750,"currency":"SITO"},"total_supply":{"sititos":100000000,"currency":"SITO"}}}`,
		},
		{name: "method", method: "POST", path: "/analytics", status: 405, code: "method_not_allowed"},
		{name: "head", method: "HEAD", path: "/analytics", status: 405, code: "method_not_allowed"},
		{name: "trailing slash", method: "GET", path: "/analytics/", status: 404, code: "not_found"},
		{name: "encoded path", method: "GET", path: "/%61nalytics", status: 404, code: "not_found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &analyticsService{report: report}
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			testRouter(service).ServeHTTP(w, req)
			if w.Code != tt.status || (tt.code != "" && !strings.Contains(w.Body.String(), `"`+tt.code+`"`)) {
				t.Fatalf("got %d %s, want %d %s", w.Code, w.Body.String(), tt.status, tt.code)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Fatalf("got %s, want %s", w.Body.String(), tt.wantBody)
			}
			if tt.status == 405 && w.Header().Get("Allow") != "GET" {
				t.Fatalf("got Allow %q", w.Header().Get("Allow"))
			}
			if tt.name == "window" {
				want := &analytics.Window{
					From: time.Date(2026, 1, 2, 1, 4, 5, 0, time.UTC),
					To:   time.Date(2026, 1, 3, 1, 4, 5, 0, time.UTC),
				}
				if !reflect.DeepEqual(service.window, want) {
					t.Fatalf("got window %+v, want %+v", service.window, want)
				}
			}
			if tt.status >= 400 && service.calls != 0 {
				t.Fatal("invalid request reached analytics service")
			}
		})
	}
}

func TestAnalyticsRejectsInvalidQueries(t *testing.T) {
	for _, query := range []string{
		"?from=2026-01-01T00:00:00Z",
		"?to=2026-01-02T00:00:00Z",
		"?from=2026-01-01T00:00:00&to=2026-01-02T00:00:00Z",
		"?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z",
		"?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z&from=2026-01-01T00:00:00Z",
		"?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z&other=x",
		"?from=%ZZ&to=2026-01-02T00:00:00Z",
		"?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z&",
		"?",
	} {
		t.Run(query, func(t *testing.T) {
			service := &analyticsService{}
			w := httptest.NewRecorder()
			testRouter(service).ServeHTTP(w, httptest.NewRequest("GET", "/analytics"+query, nil))
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"invalid_request"`) || service.calls != 0 {
				t.Fatalf("got %d %s, calls %d", w.Code, w.Body.String(), service.calls)
			}
		})
	}
}

func TestAnalyticsErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "uninitialized", err: analytics.ErrNotInitialized, status: 409, code: "ledger_not_initialized"},
		{name: "timeout", err: context.DeadlineExceeded, status: 503, code: "request_timeout"},
		{name: "storage failure", err: errors.New("private database detail"), status: 500, code: "internal_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &analyticsService{err: tt.err}
			w := httptest.NewRecorder()
			testRouter(service).ServeHTTP(w, httptest.NewRequest("GET", "/analytics", nil))
			if w.Code != tt.status || !strings.Contains(w.Body.String(), `"`+tt.code+`"`) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}
