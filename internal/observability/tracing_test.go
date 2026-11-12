package observability_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/theepsi/Ledgersito/internal/observability"
)

func TestLocalTraceParentageAndPrivacy(t *testing.T) {
	var output bytes.Buffer
	tracer := observability.NewTracer(slog.New(slog.NewJSONHandler(&output, nil)), rand.Reader)
	ctx, trace := tracer.StartRequest(context.Background(), "/readyz")
	end := observability.StartStorageSpan(ctx, "readiness")
	end(errors.New("private database error"))
	trace.End(503)

	events := spanEvents(t, output.String())
	if len(events) != 2 {
		t.Fatalf("got %d spans in %s", len(events), output.String())
	}
	child, root := events[0], events[1]
	if root["span_name"] != "http /readyz" || root["span_kind"] != "http" || root["outcome"] != "error" {
		t.Fatalf("unexpected root: %#v", root)
	}
	if child["span_name"] != "storage readiness" || child["span_kind"] != "storage" || child["outcome"] != "error" {
		t.Fatalf("unexpected child: %#v", child)
	}
	if child["trace_id"] != root["trace_id"] || child["parent_span_id"] != root["span_id"] || child["span_id"] == root["span_id"] {
		t.Fatalf("invalid parentage: %#v %#v", root, child)
	}
	for _, event := range events {
		if duration, ok := event["duration_ms"].(float64); !ok || duration < 0 {
			t.Fatalf("invalid span duration: %#v", event)
		}
		if _, ok := event["start_time"].(string); !ok {
			t.Fatalf("missing start time: %#v", event)
		}
	}
	if strings.Contains(output.String(), "private database error") {
		t.Fatal("span leaked the underlying error")
	}
}

func TestTraceIDFailureDropsEntireTrace(t *testing.T) {
	for _, tt := range []struct {
		name   string
		reader io.Reader
	}{
		{name: "root failure", reader: failingRandom{}},
		{name: "child failure", reader: io.MultiReader(bytes.NewReader(bytes.Repeat([]byte{1}, 24)), failingRandom{})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			tracer := observability.NewTracer(slog.New(slog.NewJSONHandler(&output, nil)), tt.reader)
			ctx, trace := tracer.StartRequest(context.Background(), "/readyz")
			end := observability.StartStorageSpan(ctx, "readiness")
			end(nil)
			trace.End(200)
			if got := spanEvents(t, output.String()); len(got) != 0 {
				t.Fatalf("partial trace was emitted: %s", output.String())
			}
			if !strings.Contains(output.String(), "trace ID generation failed") || strings.Contains(output.String(), "secret entropy failure") {
				t.Fatalf("failure not safely reported: %s", output.String())
			}
		})
	}
}

func TestExtraStorageSpanDropsTrace(t *testing.T) {
	var output bytes.Buffer
	tracer := observability.NewTracer(slog.New(slog.NewJSONHandler(&output, nil)), rand.Reader)
	ctx, trace := tracer.StartRequest(context.Background(), "/readyz")
	first := observability.StartStorageSpan(ctx, "readiness")
	first(nil)
	second := observability.StartStorageSpan(ctx, "readiness")
	second(nil)
	trace.End(200)
	if len(spanEvents(t, output.String())) != 0 || !strings.Contains(output.String(), "trace could not be completed") {
		t.Fatalf("unexpected trace output: %s", output.String())
	}
}

type failingRandom struct{}

func (failingRandom) Read([]byte) (int, error) {
	return 0, errors.New("secret entropy failure")
}

func TestConcurrentTracesRemainIndependent(t *testing.T) {
	var output bytes.Buffer
	tracer := observability.NewTracer(slog.New(slog.NewJSONHandler(&output, nil)), rand.Reader)
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, trace := tracer.StartRequest(context.Background(), "/readyz")
			end := observability.StartStorageSpan(ctx, "readiness")
			end(nil)
			trace.End(200)
		}()
	}
	wg.Wait()
	events := spanEvents(t, output.String())
	traces := make(map[string][]map[string]any)
	for _, event := range events {
		id, _ := event["trace_id"].(string)
		traces[id] = append(traces[id], event)
	}
	if len(traces) != 40 {
		t.Fatalf("got %d independent traces, want 40", len(traces))
	}
	for id, spans := range traces {
		if len(spans) != 2 || spans[0]["parent_span_id"] != spans[1]["span_id"] {
			t.Fatalf("invalid trace %s: %#v", id, spans)
		}
	}
}

func spanEvents(t *testing.T, output string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["event"] == "span_completed" {
			events = append(events, event)
		}
	}
	return events
}
