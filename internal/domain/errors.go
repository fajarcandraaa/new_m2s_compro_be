package domain

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ValidationError is returned by input validation (422 validation_error).
// Field keys are dot paths with numeric indices (e.g. packages.0.priceValue).
type ValidationError struct {
	Message string
	Fields  map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %v", e.Fields)
}

// ConflictError is a uniqueness or business-rule conflict (409).
// Code is "conflict" or "slug_taken".
type ConflictError struct {
	Code    string
	Message string
	Fields  map[string]string
}

func (e *ConflictError) Error() string { return e.Code + ": " + e.Message }

// SlugTaken builds the 409 slug_taken error.
func SlugTaken() *ConflictError {
	return &ConflictError{Code: "slug_taken", Message: "Slug sudah dipakai.", Fields: map[string]string{"slug": "Slug sudah dipakai."}}
}

// Common validation messages (Bahasa Indonesia).
const (
	MsgRequired     = "Wajib diisi."
	MsgInvalid      = "Nilai tidak valid."
	MsgEmail        = "Format email tidak valid."
	MsgSlug         = "Slug hanya boleh berisi huruf kecil, angka, dan tanda hubung tunggal."
	MsgHTTPURL      = "URL harus diawali http:// atau https://."
	MsgMediaURL     = "URL harus berupa /uploads/... atau diawali http:// atau https://."
	MsgPhone        = "Nomor WhatsApp tidak valid."
	MsgDate         = "Format tanggal tidak valid (RFC3339)."
	MsgTitleNoSlug  = "Judul harus mengandung huruf atau angka."
	MsgGeneric      = "Periksa kembali isian Anda."
	msgMaxLenFormat = "Maksimal %d karakter."
	msgMinLenFormat = "Minimal %d karakter."
	msgMaxItems     = "Maksimal %d item."
)

// MaxLenMsg returns the "max N characters" message.
func MaxLenMsg(n int) string { return fmt.Sprintf(msgMaxLenFormat, n) }

// MinLenMsg returns the "min N characters" message.
func MinLenMsg(n int) string { return fmt.Sprintf(msgMinLenFormat, n) }

var (
	httpURLRe  = regexp.MustCompile(`^https?://.+`)
	mediaURLRe = regexp.MustCompile(`^(/uploads/[A-Za-z0-9._-]+|https?://.+)$`)
	emailRe    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

// Validator collects field errors.
type Validator struct {
	Fields map[string]string
}

// NewValidator returns an empty validator.
func NewValidator() *Validator { return &Validator{Fields: map[string]string{}} }

// Add records msg for field unless the field already has an error.
func (v *Validator) Add(field, msg string) {
	if _, ok := v.Fields[field]; !ok {
		v.Fields[field] = msg
	}
}

// Has reports whether field already has an error.
func (v *Validator) Has(field string) bool { _, ok := v.Fields[field]; return ok }

// Str checks a (trimmed) string: required and max length in code points.
func (v *Validator) Str(field, s string, required bool, maxLen int) {
	if required && s == "" {
		v.Add(field, MsgRequired)
		return
	}
	if maxLen > 0 && utf8.RuneCountInString(s) > maxLen {
		v.Add(field, MaxLenMsg(maxLen))
	}
}

// MinLen checks a minimum length in code points (only when s is non-empty
// or when required already passed).
func (v *Validator) MinLen(field, s string, minLen int) {
	if utf8.RuneCountInString(s) < minLen {
		v.Add(field, MinLenMsg(minLen))
	}
}

// OptStr checks a nullable string's max length.
func (v *Validator) OptStr(field string, s *string, maxLen int) {
	if s != nil {
		v.Str(field, *s, false, maxLen)
	}
}

// Email checks a required email address.
func (v *Validator) Email(field, s string) {
	if s == "" {
		v.Add(field, MsgRequired)
		return
	}
	if utf8.RuneCountInString(s) > 254 {
		v.Add(field, MaxLenMsg(254))
		return
	}
	if !IsEmail(s) {
		v.Add(field, MsgEmail)
	}
}

// HTTPURL checks a nullable absolute http(s) URL (max 500).
func (v *Validator) HTTPURL(field string, s *string) {
	if s == nil {
		return
	}
	if utf8.RuneCountInString(*s) > 500 {
		v.Add(field, MaxLenMsg(500))
		return
	}
	if !httpURLRe.MatchString(*s) || strings.ContainsAny(*s, " \t\r\n") {
		v.Add(field, MsgHTTPURL)
	}
}

// MediaURL checks a media URL (site-relative /uploads/... or http(s)).
func (v *Validator) MediaURL(field string, s *string, required bool) {
	if s == nil || *s == "" {
		if required {
			v.Add(field, MsgRequired)
		}
		return
	}
	if utf8.RuneCountInString(*s) > 500 {
		v.Add(field, MaxLenMsg(500))
		return
	}
	if !mediaURLRe.MatchString(*s) || strings.ContainsAny(*s, " \t\r\n") {
		v.Add(field, MsgMediaURL)
	}
}

// StrList checks a list of strings: count and per-item required/max length.
func (v *Validator) StrList(field string, list []string, maxItems, maxLen int) {
	if len(list) > maxItems {
		v.Add(field, fmt.Sprintf(msgMaxItems, maxItems))
		return
	}
	for i, s := range list {
		v.Str(fmt.Sprintf("%s.%d", field, i), s, true, maxLen)
	}
}

// MaxItems checks only the number of items.
func (v *Validator) MaxItems(field string, n, maxItems int) bool {
	if n > maxItems {
		v.Add(field, fmt.Sprintf(msgMaxItems, maxItems))
		return false
	}
	return true
}

// Enum checks that s is one of allowed.
func (v *Validator) Enum(field, s string, allowed ...string) {
	for _, a := range allowed {
		if s == a {
			return
		}
	}
	if s == "" {
		v.Add(field, MsgRequired)
		return
	}
	v.Add(field, MsgInvalid)
}

// Seo validates SEO overrides under prefix "seo".
func (v *Validator) Seo(s Seo) {
	v.OptStr("seo.title", s.Title, 70)
	v.OptStr("seo.description", s.Description, 160)
	v.MediaURL("seo.ogImageUrl", s.OgImageURL, false)
}

// Err returns a *ValidationError when any field failed, else nil.
func (v *Validator) Err() error {
	if len(v.Fields) == 0 {
		return nil
	}
	return &ValidationError{Message: MsgGeneric, Fields: v.Fields}
}

// IsEmail is a pragmatic email check.
func IsEmail(s string) bool {
	if !emailRe.MatchString(s) {
		return false
	}
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// Trim trims surrounding whitespace.
func Trim(s string) string { return strings.TrimSpace(s) }

// TrimPtr trims a nullable string; "" becomes nil.
func TrimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// TrimList trims every element and never returns nil.
func TrimList(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

// TrimSeo trims SEO fields (""→nil).
func TrimSeo(s Seo) Seo {
	return Seo{Title: TrimPtr(s.Title), Description: TrimPtr(s.Description), OgImageURL: TrimPtr(s.OgImageURL)}
}
