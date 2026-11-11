package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

func (h *Handler) serveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, 400, "invalid_request", "Query parameters are not supported.")
		return
	}
	h.write(w, 200, []byte(`{"status":"ok"}`))
}

func (h *Handler) serveReadiness(w http.ResponseWriter, r *http.Request) {
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
	if h.readiness == nil {
		h.readinessError(w, errors.New("readiness checker unavailable"))
		return
	}
	if err := h.readiness.CheckReadiness(ctx); err != nil {
		h.readinessError(w, err)
		return
	}
	if ctx.Err() != nil {
		h.readinessError(w, ctx.Err())
		return
	}
	h.write(w, 200, []byte(`{"status":"ready"}`))
}

func (h *Handler) readinessError(w http.ResponseWriter, err error) {
	h.logger.Error("readiness check failed", "error", err)
	h.failure(w, 503, "not_ready", "Service is not ready.")
}
