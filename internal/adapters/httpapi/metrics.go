package httpapi

import "net/http"

func (h *Handler) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, 400, "invalid_request", "Query parameters are not supported.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if _, err := w.Write(h.metrics.Render()); err != nil {
		h.logger.Warn("write HTTP response failed", "error", err)
	}
}
