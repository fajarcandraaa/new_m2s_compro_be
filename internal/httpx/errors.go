// Package httpx contains the HTTP plumbing shared by all handlers: the error
// type → status adapter, JSON envelopes, request decoding, query parsing,
// client-IP derivation and the generic middlewares.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

// Error is an HTTP error with a stable code.
type Error struct {
	Status     int
	Code       string
	Message    string
	Fields     map[string]string
	RetryAfter int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Constructors for the codes listed in components/schemas/Error.
func BadRequest(msg string) *Error {
	if msg == "" {
		msg = "Permintaan tidak valid."
	}
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func Unauthorized(msg string) *Error {
	if msg == "" {
		msg = "Sesi tidak valid. Silakan masuk kembali."
	}
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: msg}
}

func Forbidden() *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "Anda tidak memiliki akses untuk tindakan ini."}
}

func CSRFFailed() *Error {
	return &Error{Status: http.StatusForbidden, Code: "csrf_failed", Message: "Permintaan ditolak karena header keamanan tidak ada."}
}

func NotFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: "Data tidak ditemukan."}
}

func MethodNotAllowed() *Error {
	return &Error{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Metode tidak diizinkan."}
}

func PayloadTooLarge() *Error {
	return &Error{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: "Ukuran data terlalu besar."}
}

func UnsupportedMediaType() *Error {
	return &Error{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Jenis file tidak didukung. Gunakan JPG, PNG, atau WebP."}
}

func RateLimited(retryAfter int) *Error {
	if retryAfter < 1 {
		retryAfter = 1
	}
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Terlalu banyak permintaan. Coba lagi beberapa saat lagi.", RetryAfter: retryAfter}
}

func internalError() *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "Terjadi kesalahan pada server."}
}

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// toHTTP maps any error to an *Error.
func toHTTP(err error) (*Error, bool) {
	var he *Error
	if errors.As(err, &he) {
		return he, true
	}
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		msg := ve.Message
		if msg == "" {
			msg = domain.MsgGeneric
		}
		return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_error", Message: msg, Fields: ve.Fields}, true
	}
	var ce *domain.ConflictError
	if errors.As(err, &ce) {
		code := ce.Code
		if code == "" {
			code = "conflict"
		}
		return &Error{Status: http.StatusConflict, Code: code, Message: ce.Message, Fields: ce.Fields}, true
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound(), true
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return PayloadTooLarge(), true
	}
	var pge *pgconn.PgError
	if errors.As(err, &pge) && pge.Code == "23505" {
		switch {
		case strings.Contains(pge.ConstraintName, "slug"):
			return toHTTPMust(domain.SlugTaken()), true
		case strings.Contains(pge.ConstraintName, "email"):
			return &Error{Status: http.StatusConflict, Code: "conflict", Message: "Email sudah dipakai.", Fields: map[string]string{"email": "Email sudah dipakai."}}, true
		default:
			return &Error{Status: http.StatusConflict, Code: "conflict", Message: "Data bentrok dengan data lain."}, true
		}
	}
	return internalError(), false
}

func toHTTPMust(ce *domain.ConflictError) *Error {
	return &Error{Status: http.StatusConflict, Code: ce.Code, Message: ce.Message, Fields: ce.Fields}
}

// WriteError writes the error envelope. Unknown errors are logged and
// returned as 500 internal_error with a generic message.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	he, known := toHTTP(err)
	if !known {
		slog.ErrorContext(r.Context(), "internal error", "err", err.Error(), "method", r.Method, "path", r.URL.Path)
	}
	if he.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(he.RetryAfter))
	}
	fields := he.Fields
	if len(fields) == 0 {
		fields = nil
	}
	JSON(w, he.Status, errorBody{Error: errorPayload{Code: he.Code, Message: he.Message, Fields: fields}})
}

// HandlerFunc is a handler that returns an error instead of writing it.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// H adapts a HandlerFunc to http.HandlerFunc; this adapter is the only place
// that writes error responses for handlers.
func H(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}
