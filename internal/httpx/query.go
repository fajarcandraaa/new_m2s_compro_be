package httpx

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID string.
func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// PathID returns the {id} path parameter or 400 when it is not a UUID.
func PathID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !IsUUID(id) {
		return "", BadRequest("ID tidak valid.")
	}
	return strings.ToLower(id), nil
}

// PathSlug returns the {slug} path parameter or 404 when it does not match
// the slug pattern.
func PathSlug(r *http.Request) (string, error) {
	slug := chi.URLParam(r, "slug")
	if !domain.IsSlug(slug) {
		return "", NotFound()
	}
	return slug, nil
}

// query returns a trimmed query value ("" = absent).
func query(r *http.Request, name string) string {
	return strings.TrimSpace(r.URL.Query().Get(name))
}

// QueryInt parses an optional integer in [min,max]; absent → def.
func QueryInt(r *http.Request, name string, min, max, def int) (int, error) {
	raw := query(r, name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, BadRequest("Parameter " + name + " tidak valid.")
	}
	return n, nil
}

// Page parses page (>=1, default 1) and pageSize (1..100, default 20).
func Page(r *http.Request) (page, size int, err error) {
	page, err = QueryInt(r, "page", 1, 1<<30, 1)
	if err != nil {
		return 0, 0, err
	}
	size, err = QueryInt(r, "pageSize", 1, 100, 20)
	if err != nil {
		return 0, 0, err
	}
	return page, size, nil
}

// QueryBool parses an optional boolean (only "true"/"false").
func QueryBool(r *http.Request, name string) (*bool, error) {
	switch query(r, name) {
	case "":
		return nil, nil
	case "true":
		b := true
		return &b, nil
	case "false":
		b := false
		return &b, nil
	}
	return nil, BadRequest("Parameter " + name + " tidak valid.")
}

// QueryEnum parses an optional enum value.
func QueryEnum(r *http.Request, name string, allowed ...string) (string, error) {
	v := query(r, name)
	if v == "" {
		return "", nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", BadRequest("Parameter " + name + " tidak valid.")
}

// QueryString parses an optional string with a max length in code points.
func QueryString(r *http.Request, name string, maxLen int) (string, error) {
	v := query(r, name)
	if utf8.RuneCountInString(v) > maxLen {
		return "", BadRequest("Parameter " + name + " terlalu panjang.")
	}
	return v, nil
}

// QuerySlug parses an optional slug-shaped query value.
func QuerySlug(r *http.Request, name string) (string, error) {
	v := query(r, name)
	if v == "" {
		return "", nil
	}
	if !domain.IsSlug(v) {
		return "", BadRequest("Parameter " + name + " tidak valid.")
	}
	return v, nil
}
