package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/theepsi/Ledgersito/internal/application/history"
)

type HistoryService interface {
	List(context.Context) ([]history.Transaction, error)
}

func (h *Handler) serveHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, 400, "invalid_request", "Query parameters are not supported.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	items, err := h.history.List(ctx)
	if err != nil {
		h.historyError(w, ctx, err)
		return
	}
	if items == nil {
		items = []history.Transaction{}
	}
	body, err := json.Marshal(struct {
		Transactions []history.Transaction `json:"transactions"`
	}{items})
	if err != nil {
		h.historyError(w, ctx, err)
		return
	}
	if ctx.Err() != nil {
		h.historyError(w, ctx, ctx.Err())
		return
	}
	h.write(w, 200, body)
}

func (h *Handler) historyError(w http.ResponseWriter, ctx context.Context, err error) {
	h.logger.Error("transaction history operation failed", slog.Any("error", err))
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		h.failure(w, 503, "request_timeout", "Operation timed out.")
		return
	}
	h.failure(w, 500, "internal_error", "An internal error occurred.")
}
