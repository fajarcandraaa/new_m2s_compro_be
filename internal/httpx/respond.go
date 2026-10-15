package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// PageMeta is the list metadata.
type PageMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

// Unpaginated returns meta for unpaginated / limit-based lists.
func Unpaginated(n int) PageMeta {
	size := n
	if size < 1 {
		size = 1
	}
	return PageMeta{Page: 1, PageSize: size, Total: n}
}

type dataEnvelope struct {
	Data any `json:"data"`
}

type listEnvelope struct {
	Data any      `json:"data"`
	Meta PageMeta `json:"meta"`
}

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"Terjadi kesalahan pada server."}}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// Data writes {"data": v}.
func Data(w http.ResponseWriter, status int, v any) {
	JSON(w, status, dataEnvelope{Data: v})
}

// List writes {"data": items, "meta": meta}.
func List(w http.ResponseWriter, items any, meta PageMeta) {
	JSON(w, http.StatusOK, listEnvelope{Data: items, Meta: meta})
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// MaxJSONBody is the JSON request body cap (1 MiB).
const MaxJSONBody = 1 << 20

// Decode decodes a JSON request body into dst: body capped at 1 MiB (413),
// unknown fields and malformed JSON → 400, trailing data → 400.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return PayloadTooLarge()
		}
		if errors.Is(err, io.EOF) {
			return BadRequest("Body JSON wajib diisi.")
		}
		return BadRequest("JSON tidak valid atau berisi field yang tidak dikenal.")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return PayloadTooLarge()
		}
		return BadRequest("JSON tidak valid.")
	}
	return nil
}

// Optional tracks presence for PATCH bodies: Set is true when the key is
// present, Null when its value is JSON null.
type Optional[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(bytes.TrimSpace(b)) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

// Time marshals as RFC3339 in UTC (no fractional seconds).
type Time time.Time

// MarshalJSON implements json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format(time.RFC3339) + `"`), nil
}

// TimePtr converts a nullable time.
func TimePtr(t *time.Time) *Time {
	if t == nil {
		return nil
	}
	v := Time(*t)
	return &v
}
