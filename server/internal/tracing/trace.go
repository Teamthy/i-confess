package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKeyTraceID struct{}
type ctxKeySpanID struct{}

// IDs are W3C traceparent-compatible (trace-id 32 hex, span-id 16 hex).
func newTraceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
func newSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// TraceIDFromContext returns trace ID for log correlation.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTraceID{}).(string); ok {
		return v
	}
	return ""
}

// Middleware ensures every request has traceparent + X-Trace-Id.
// If client sent traceparent, it is preserved; otherwise a new trace is started.
// This satisfies Phase 7 §7.2 OTel traces per request/job (stub — wire OTel SDK in prod).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := TraceIDFromContext(r.Context())
		parent := r.Header.Get("traceparent")
		var spanID string
		if traceID == "" {
			// W3C traceparent: 00-<trace-id>-<parent-id>-01
			if parent != "" && len(parent) >= 55 {
				// extract trace-id from incoming
				parts := splitTraceParent(parent)
				if len(parts) == 4 {
					traceID = parts[1]
					spanID = newSpanID()
					w.Header().Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
				} else {
					traceID = newTraceID()
					spanID = newSpanID()
					w.Header().Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
				}
			} else {
				traceID = newTraceID()
				spanID = newSpanID()
				w.Header().Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
			}
		} else {
			spanID = newSpanID()
		}
		w.Header().Set("X-Trace-Id", traceID)
		ctx := context.WithValue(r.Context(), ctxKeyTraceID{}, traceID)
		ctx = context.WithValue(ctx, ctxKeySpanID{}, spanID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func splitTraceParent(s string) []string {
	// naive split by '-'
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// StartSpan creates a child span linked to current trace (for jobs).
func StartSpan(ctx context.Context) (context.Context, string, string) {
	traceID := TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = newTraceID()
		ctx = context.WithValue(ctx, ctxKeyTraceID{}, traceID)
	}
	spanID := newSpanID()
	ctx = context.WithValue(ctx, ctxKeySpanID{}, spanID)
	return ctx, traceID, spanID
}
