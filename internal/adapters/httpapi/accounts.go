package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/observability"
)

type Service interface {
	Create(context.Context, string, string) (accounts.Response, error)
	List(context.Context) ([]accounts.Account, error)
}

type Handler struct {
	service   Service
	logger    *slog.Logger
	transfers TransferService
	history   HistoryService
	analytics AnalyticsService
	readiness ReadinessChecker
	metrics   *observability.Metrics
	tracer    *observability.Tracer
}

func New(service Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) WithMetrics(metrics *observability.Metrics) *Handler {
	h.metrics = metrics
	return h
}

func (h *Handler) WithTracer(tracer *observability.Tracer) *Handler {
	h.tracer = tracer
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	route := h.routeLabel(r)
	var trace *observability.Trace
	if h.tracer != nil {
		ctx, startedTrace := h.tracer.StartRequest(r.Context(), route)
		trace = startedTrace
		r = r.WithContext(ctx)
	}
	tracked := &responseStatusWriter{ResponseWriter: w}
	h.serveHTTP(tracked, r)
	status := tracked.status
	if status == 0 {
		status = http.StatusOK
	}
	if h.metrics != nil && (r.URL.Path != "/metrics" || r.URL.RawPath != "") {
		h.metrics.ObserveHTTP(r.Method, route, status, time.Since(started))
	}
	trace.End(status)
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	} else if status >= 400 {
		level = slog.LevelWarn
	}
	h.logger.LogAttrs(context.Background(), level, "http request completed",
		slog.String("event", "http_request_completed"),
		slog.String("method", r.Method),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Float64("duration_ms", float64(time.Since(started))/float64(time.Millisecond)),
	)
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/metrics" && r.URL.RawPath == "" && h.metrics != nil {
		h.serveMetrics(w, r)
		return
	}
	if r.URL.Path == "/healthz" && r.URL.RawPath == "" {
		h.serveHealth(w, r)
		return
	}
	if r.URL.Path == "/readyz" && r.URL.RawPath == "" {
		h.serveReadiness(w, r)
		return
	}
	if r.URL.Path == "/transfers" && r.URL.RawPath == "" && h.transfers != nil {
		h.serveTransfer(w, r)
		return
	}
	if r.URL.Path == "/transactions" && r.URL.RawPath == "" && h.history != nil {
		h.serveHistory(w, r)
		return
	}
	if r.URL.Path == "/analytics" && r.URL.RawPath == "" && h.analytics != nil {
		h.serveAnalytics(w, r)
		return
	}
	if r.URL.Path != "/accounts" || r.URL.RawPath != "" {
		h.failure(w, 404, "not_found", "Route not found.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, 400, "invalid_request", "Query parameters are not supported.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		items, err := h.service.List(ctx)
		if err != nil {
			h.applicationError(w, ctx, err)
			return
		}
		if items == nil {
			items = []accounts.Account{}
		}
		body, err := json.Marshal(struct {
			Accounts []accounts.Account `json:"accounts"`
		}{items})
		if err != nil {
			h.applicationError(w, ctx, err)
			return
		}
		if ctx.Err() != nil {
			h.applicationError(w, ctx, ctx.Err())
			return
		}
		h.write(w, 200, body)
		return
	}
	body, key, ok := h.readPost(w, r)
	if !ok {
		return
	}
	name, err := decodeName(body)
	if err != nil {
		h.failure(w, 400, "invalid_request", "Provide exactly one JSON name string.")
		return
	}
	name, err = account.NormalizeName(name)
	if err != nil {
		h.failure(w, 400, "invalid_name", "Name must contain 1-100 Unicode code points after trimming, without control characters.")
		return
	}
	reply, err := h.service.Create(ctx, key, name)
	if err != nil {
		h.applicationError(w, ctx, err)
		return
	}
	if ctx.Err() != nil {
		h.applicationError(w, ctx, ctx.Err())
		return
	}
	h.write(w, reply.Status, reply.Body)
}

func (h *Handler) routeLabel(r *http.Request) string {
	if r.URL.RawPath != "" {
		return "unmatched"
	}
	switch r.URL.Path {
	case "/accounts":
		return "/accounts"
	case "/transfers":
		if h.transfers != nil {
			return "/transfers"
		}
	case "/transactions":
		if h.history != nil {
			return "/transactions"
		}
	case "/analytics":
		if h.analytics != nil {
			return "/analytics"
		}
	case "/healthz":
		return "/healthz"
	case "/readyz":
		return "/readyz"
	}
	return "unmatched"
}

type responseStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseStatusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (h *Handler) readPost(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) == 0 {
		h.failure(w, 400, "missing_idempotency_key", "An Idempotency-Key is required.")
		return nil, "", false
	}
	if len(keys) != 1 || !validKey(keys[0]) {
		h.failure(w, 400, "invalid_idempotency_key", "Provide one key of 1-128 ASCII letters, digits, hyphens, or underscores.")
		return nil, "", false
	}
	mediaValues := r.Header.Values("Content-Type")
	if len(mediaValues) != 1 || !validMedia(mediaValues[0]) {
		h.failure(w, 415, "unsupported_media_type", "Content-Type must be application/json with optional UTF-8 charset.")
		return nil, "", false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.failure(w, 413, "request_too_large", "Request body exceeds 4096 bytes.")
		} else {
			h.logger.Warn("read request body failed", "error", err)
			h.failure(w, 400, "invalid_request", "Unable to read request body.")
		}
		return nil, "", false
	}
	return body, keys[0], true
}

func validKey(key string) bool {
	if len(key) < 1 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func validMedia(value string) bool {
	media, params, err := mime.ParseMediaType(value)
	if err != nil || media != "application/json" {
		return false
	}
	for key, value := range params {
		if key != "charset" || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func decodeName(body []byte) (string, error) {
	fields, err := decodeObject(body, "name")
	if err != nil {
		return "", err
	}
	return decodeString(fields["name"])
}

func decodeString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || raw[0] != '"' || !validSurrogates(raw) {
		return "", errors.New("invalid JSON string")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func decodeObject(body []byte, names ...string) (map[string]json.RawMessage, error) {
	invalid := errors.New("invalid JSON object")
	if !utf8.Valid(body) {
		return nil, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	fields := make(map[string]json.RawMessage, len(names))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		name, ok := token.(string)
		allowed := false
		for _, expected := range names {
			if name == expected {
				allowed = true
			}
		}
		if !ok || !allowed || fields[name] != nil {
			return nil, invalid
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, invalid
		}
		fields[name] = raw
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || len(fields) != len(names) {
		return nil, invalid
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid
	}
	return fields, nil
}

// encoding/json replaces unpaired UTF-16 surrogates; reject them instead of
// silently changing strings used for idempotency.
func validSurrogates(raw []byte) bool {
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xDC00 && n <= 0xDFFF {
			return false
		}
		if n >= 0xD800 && n <= 0xDBFF {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return false
			}
			i += 6
		}
	}
	return true
}

func (h *Handler) applicationError(w http.ResponseWriter, ctx context.Context, err error) {
	h.logger.Error("account operation failed", "error", err)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		h.failure(w, 503, "request_timeout", "Operation timed out. Retry creation with the same idempotency key.")
	case errors.Is(err, accounts.ErrKeyReused):
		h.failure(w, 422, "idempotency_key_reused", "Key was already used with a different name.")
	case errors.Is(err, account.ErrInvalidName):
		h.failure(w, 400, "invalid_name", "Invalid account name.")
	default:
		h.failure(w, 500, "internal_error", "An internal error occurred.")
	}
}

func (h *Handler) failure(w http.ResponseWriter, status int, code, message string) {
	body, err := json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{code, message}})
	if err != nil {
		h.logger.Error("encode error response failed", "error", err)
		http.Error(w, "Internal error", 500)
		return
	}
	h.write(w, status, body)
}

func (h *Handler) write(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		h.logger.Warn("write HTTP response failed", "error", err)
	}
}
