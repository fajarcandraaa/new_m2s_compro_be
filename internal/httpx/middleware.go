package httpx

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type ctxKey int

const clientIPKey ctxKey = iota

// ClientIP derives the client IP: the X-Forwarded-For entry `hops`
// positions from the right; if XFF has fewer entries (or hops is 0) the
// RemoteAddr is used. The port is stripped.
func ClientIP(r *http.Request, hops int) string {
	var entries []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(h, ",") {
			if p := strings.TrimSpace(part); p != "" {
				entries = append(entries, p)
			}
		}
	}
	if hops > 0 && len(entries) >= hops {
		return stripPort(entries[len(entries)-hops])
	}
	return stripPort(r.RemoteAddr)
}

func stripPort(s string) string {
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return strings.Trim(s, "[]")
}

// ClientIPMiddleware stores the derived client IP in the request context.
func ClientIPMiddleware(hops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r, hops)
			slog.DebugContext(r.Context(), "client ip",
				"xff", r.Header.Values("X-Forwarded-For"), "remote_addr", r.RemoteAddr, "client_ip", ip)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

// IPFromContext returns the client IP stored by ClientIPMiddleware.
func IPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// Recoverer logs panics and returns 500 internal_error.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.ErrorContext(r.Context(), "panic", "panic", rec, "stack", string(debug.Stack()))
				WriteError(w, r, internalError())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// AccessLog logs one line per request (never bodies or cookies).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		slog.InfoContext(r.Context(), "request",
			"method", r.Method, "path", r.URL.Path, "status", status,
			"duration_ms", time.Since(start).Milliseconds(), "bytes", ww.BytesWritten())
	})
}

// LogHandler wraps a slog.Handler and adds request_id from the context.
type LogHandler struct{ slog.Handler }

// Handle implements slog.Handler.
func (h LogHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id := middleware.GetReqID(ctx); id != "" {
		rec.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, rec)
}

// WithAttrs implements slog.Handler.
func (h LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return LogHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler.
func (h LogHandler) WithGroup(name string) slog.Handler {
	return LogHandler{h.Handler.WithGroup(name)}
}

// JSONNotFound is the chi NotFound handler.
func JSONNotFound(w http.ResponseWriter, r *http.Request) { WriteError(w, r, NotFound()) }

// JSONMethodNotAllowed is the chi MethodNotAllowed handler.
func JSONMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, MethodNotAllowed())
}

// NoStore marks responses as non-cacheable (admin and auth routes).
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
