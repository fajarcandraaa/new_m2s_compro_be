package domain

import (
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// SlugMaxLen is the maximum slug length.
const SlugMaxLen = 120

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// IsSlug reports whether s matches the slug pattern and length.
func IsSlug(s string) bool {
	return len(s) > 0 && len(s) <= SlugMaxLen && slugRe.MatchString(s)
}

// Slugify implements the shared slug rule from the contract:
// lowercase → NFKD and drop combining marks → `&` becomes ` dan ` → every
// run of characters outside [a-z0-9] becomes `-` → trim `-` → cut to 120
// characters and trim a trailing `-`.
func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	s = strings.ReplaceAll(b.String(), "&", " dan ")

	var out strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	res := strings.Trim(out.String(), "-")
	return TruncateSlug(res, SlugMaxLen)
}

// TruncateSlug cuts slug to max bytes (slugs are ASCII) and trims a
// trailing hyphen.
func TruncateSlug(slug string, max int) string {
	if len(slug) > max {
		slug = slug[:max]
	}
	return strings.TrimRight(slug, "-")
}

var e164Re = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// NormalizePhone converts a phone number as typed into E.164: strip spaces,
// `(`, `)`, `-`; leading `0` → `+62`; leading `62` → `+62`. ok is false when
// the result is not valid E.164.
func NormalizePhone(raw string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '(', ')', '-', '\t':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(s, "0"):
		s = "+62" + s[1:]
	case strings.HasPrefix(s, "62"):
		s = "+" + s
	}
	if !e164Re.MatchString(s) {
		return "", false
	}
	return s, true
}

// IsE164 reports whether s is a valid E.164 number.
func IsE164(s string) bool { return e164Re.MatchString(s) }

// Promo statuses.
const (
	PromoActive   = "active"
	PromoUpcoming = "upcoming"
	PromoEnded    = "ended"
)

// PromoPublicWindow is how long an ended promo stays publicly visible.
const PromoPublicWindow = 7 * 24 * time.Hour

// PromoStatus computes the promo status: upcoming (now < startsAt), active
// (startsAt <= now < endsAt), ended (now >= endsAt). endsAt is exclusive.
func PromoStatus(now, startsAt, endsAt time.Time) string {
	switch {
	case now.Before(startsAt):
		return PromoUpcoming
	case now.Before(endsAt):
		return PromoActive
	default:
		return PromoEnded
	}
}

// PromoPubliclyVisible reports whether a promo is shown on the public site
// (ended promos stay visible for 7 days after endsAt).
func PromoPubliclyVisible(now, endsAt time.Time) bool {
	return endsAt.After(now.Add(-PromoPublicWindow))
}

// WordsPerMinute is the reading speed used for readMinutes.
const WordsPerMinute = 200

// PlainText extracts the text content of an HTML fragment.
func PlainText(fragment string) string {
	z := html.NewTokenizer(strings.NewReader(fragment))
	var b strings.Builder
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			// io.EOF or a tokenizer error: return what we have.
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			// Separate words across block boundaries.
			b.WriteByte(' ')
		}
	}
}

// ReadMinutes computes the reading time from the word count of the stripped
// HTML at 200 wpm, rounded up, minimum 1.
func ReadMinutes(contentHTML string) int {
	words := len(strings.Fields(PlainText(contentHTML)))
	m := int(math.Ceil(float64(words) / WordsPerMinute))
	if m < 1 {
		return 1
	}
	return m
}

var (
	ugc    = bluemonday.UGCPolicy()
	strict = bluemonday.StrictPolicy()
)

// SanitizeHTML sanitises rich text with the bluemonday UGC policy.
func SanitizeHTML(s string) string { return ugc.Sanitize(s) }

// StripHTML turns s into plain text: the bluemonday strict policy drops
// every tag and the entities it emits are unescaped again (`&` stays `&`).
// It repeats until the text is stable so that saving the result again
// gives the same value (`&lt;b&gt;` must not come back as a tag).
func StripHTML(s string) string {
	for i := 0; i < 5; i++ {
		out := html.UnescapeString(strict.Sanitize(s))
		if out == s {
			break
		}
		s = out
	}
	return s
}
