package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	appanalytics "github.com/theepsi/Ledgersito/internal/application/analytics"
)

type AnalyticsService interface {
	Execute(context.Context, *appanalytics.Window) (appanalytics.Report, error)
}

type analyticsEnvelope struct {
	Period            any                            `json:"period"`
	Transfers         appanalytics.Transfers         `json:"transfers"`
	TimestampCoverage appanalytics.TimestampCoverage `json:"timestamp_coverage"`
	Current           appanalytics.Current           `json:"current"`
}

func (h *Handler) serveAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	window, err := parseAnalyticsWindow(r.URL)
	if err != nil {
		h.failure(w, 400, "invalid_request", "Provide both from and to as RFC 3339 timestamps with an explicit UTC offset, where from is earlier than to.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	report, err := h.analytics.Execute(ctx, window)
	if err != nil {
		h.analyticsError(w, ctx, err)
		return
	}
	period := any("lifetime")
	if report.Window != nil {
		period = struct {
			From string `json:"from"`
			To   string `json:"to"`
		}{
			From: report.Window.From.UTC().Format(time.RFC3339Nano),
			To:   report.Window.To.UTC().Format(time.RFC3339Nano),
		}
	}
	body, err := json.Marshal(analyticsEnvelope{
		Period: period, Transfers: report.Transfers,
		TimestampCoverage: report.TimestampCoverage, Current: report.Current,
	})
	if err != nil {
		h.analyticsError(w, ctx, err)
		return
	}
	if ctx.Err() != nil {
		h.analyticsError(w, ctx, ctx.Err())
		return
	}
	h.write(w, 200, body)
}

func parseAnalyticsWindow(u *url.URL) (*appanalytics.Window, error) {
	if u.ForceQuery && u.RawQuery == "" {
		return nil, errors.New("empty query delimiter")
	}
	if u.RawQuery == "" {
		return nil, nil
	}
	if strings.HasPrefix(u.RawQuery, "&") || strings.HasSuffix(u.RawQuery, "&") || strings.Contains(u.RawQuery, "&&") {
		return nil, errors.New("empty query parameter")
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(values) != 2 || len(values["from"]) != 1 || len(values["to"]) != 1 {
		return nil, errors.New("invalid analytics query")
	}
	from, err := time.Parse(time.RFC3339Nano, values["from"][0])
	if err != nil {
		return nil, errors.New("invalid from timestamp")
	}
	to, err := time.Parse(time.RFC3339Nano, values["to"][0])
	if err != nil || !from.Before(to) {
		return nil, errors.New("invalid to timestamp")
	}
	return &appanalytics.Window{From: from.UTC(), To: to.UTC()}, nil
}

func (h *Handler) analyticsError(w http.ResponseWriter, ctx context.Context, err error) {
	h.logger.Error("ledger analytics operation failed", slog.Any("error", err))
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		h.failure(w, 503, "request_timeout", "Operation timed out.")
	case errors.Is(err, appanalytics.ErrNotInitialized):
		h.failure(w, 409, "ledger_not_initialized", "Initialize the ledger before requesting analytics.")
	default:
		h.failure(w, 500, "internal_error", "An internal error occurred.")
	}
}
