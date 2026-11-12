package observability

import (
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"sync"
	"time"
)

type Tracer struct {
	logger *slog.Logger
	random io.Reader
	mu     sync.Mutex
}

func NewTracer(logger *slog.Logger, random io.Reader) *Tracer {
	return &Tracer{logger: logger, random: random}
}

type traceKey struct{}

type span struct {
	id, parent, kind, name, outcome string
	start                           time.Time
	duration                        time.Duration
}

type Trace struct {
	tracer       *Tracer
	id           string
	root         span
	mu           sync.Mutex
	child        *span
	childStarted bool
	failed       bool
	ended        bool
}

func (t *Tracer) randomID(size int, different string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for range 4 {
		data := make([]byte, size)
		if _, err := io.ReadFull(t.random, data); err != nil {
			return "", false
		}
		id := hex.EncodeToString(data)
		nonzero := false
		for _, b := range data {
			nonzero = nonzero || b != 0
		}
		if nonzero && id != different {
			return id, true
		}
	}
	return "", false
}

func (t *Tracer) StartRequest(ctx context.Context, route string) (context.Context, *Trace) {
	traceID, ok := t.randomID(16, "")
	if !ok {
		t.logger.Error("trace ID generation failed")
		return ctx, nil
	}
	spanID, ok := t.randomID(8, "")
	if !ok {
		t.logger.Error("trace ID generation failed")
		return ctx, nil
	}
	switch route {
	case "/accounts", "/transfers", "/transactions", "/analytics", "/healthz", "/readyz":
	default:
		route = "unmatched"
	}
	trace := &Trace{
		tracer: t,
		id:     traceID,
		root:   span{id: spanID, kind: "http", name: "http " + route, start: time.Now()},
	}
	return context.WithValue(ctx, traceKey{}, trace), trace
}

func StartStorageSpan(ctx context.Context, operation string) func(error) {
	trace, ok := ctx.Value(traceKey{}).(*Trace)
	if !ok {
		return func(error) {}
	}
	switch operation {
	case "create_account", "list_accounts", "transfer", "list_transactions", "analytics", "readiness":
	default:
		operation = "other"
	}

	trace.mu.Lock()
	if trace.failed || trace.ended {
		trace.mu.Unlock()
		return func(error) {}
	}
	if trace.childStarted {
		trace.failed = true
		trace.mu.Unlock()
		trace.tracer.logger.Error("trace could not be completed")
		return func(error) {}
	}
	trace.childStarted = true
	trace.mu.Unlock()

	id, ok := trace.tracer.randomID(8, trace.root.id)
	if !ok {
		trace.mu.Lock()
		trace.failed = true
		trace.mu.Unlock()
		trace.tracer.logger.Error("trace ID generation failed")
		return func(error) {}
	}
	child := span{id: id, parent: trace.root.id, kind: "storage", name: "storage " + operation, start: time.Now()}
	var once sync.Once
	return func(err error) {
		once.Do(func() {
			child.duration = time.Since(child.start)
			child.outcome = "ok"
			if err != nil {
				child.outcome = "error"
			}
			trace.mu.Lock()
			trace.child = &child
			trace.mu.Unlock()
		})
	}
}

func (t *Trace) End(status int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.ended {
		t.mu.Unlock()
		return
	}
	t.ended = true
	if t.failed {
		t.mu.Unlock()
		return
	}
	if t.childStarted && t.child == nil {
		t.mu.Unlock()
		t.tracer.logger.Error("trace could not be completed")
		return
	}
	root := t.root
	root.duration = time.Since(root.start)
	root.outcome = "ok"
	if status >= 500 {
		root.outcome = "error"
	}
	child := t.child
	t.mu.Unlock()
	if child != nil {
		t.emit(*child)
	}
	t.emit(root)
}

func (t *Trace) emit(s span) {
	attrs := []slog.Attr{
		slog.String("event", "span_completed"),
		slog.String("trace_id", t.id),
		slog.String("span_id", s.id),
		slog.String("span_kind", s.kind),
		slog.String("span_name", s.name),
		slog.Time("start_time", s.start.UTC()),
		slog.Float64("duration_ms", float64(s.duration)/float64(time.Millisecond)),
		slog.String("outcome", s.outcome),
	}
	if s.parent != "" {
		attrs = append(attrs, slog.String("parent_span_id", s.parent))
	}
	t.tracer.logger.LogAttrs(context.Background(), slog.LevelInfo, "trace span completed", attrs...)
}
