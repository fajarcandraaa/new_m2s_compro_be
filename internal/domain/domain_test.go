package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"AI & Teknologi":                  "ai-dan-teknologi",
		"Keamanan Data":                   "keamanan-data",
		"  Café Ñandú  ":                  "cafe-nandu",
		"UI/UX Design":                    "ui-ux-design",
		"Data Analysis & Processing":      "data-analysis-dan-processing",
		"!!!":                             "",
		"Ketika data karyawan --- tidak!": "ketika-data-karyawan-tidak",
		"AI Bisa Membuat Website. Tapi?":  "ai-bisa-membuat-website-tapi",
		"Rp 3.500.000":                    "rp-3-500-000",
		"ＦＵＬＬＷＩＤＴＨ":                       "fullwidth",
		"&":                               "dan",
		"Über—Straße 2026":                "uber-stra-e-2026",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := Slugify(strings.Repeat("abc ", 60))
	if len(long) > SlugMaxLen || strings.HasSuffix(long, "-") || !IsSlug(long) {
		t.Errorf("long slug invalid: %q (%d)", long, len(long))
	}
}

func TestIsSlug(t *testing.T) {
	for s, ok := range map[string]bool{"a": true, "a-b-1": true, "": false, "A": false, "a--b": false, "-a": false, "a-": false, strings.Repeat("a", 121): false} {
		if IsSlug(s) != ok {
			t.Errorf("IsSlug(%q) != %v", s, ok)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	ok := map[string]string{
		"081234567890":      "+6281234567890",
		"0812-3456-7890":    "+6281234567890",
		"6281234567890":     "+6281234567890",
		"+62 812 3456 7890": "+6281234567890",
		"+6282230109923":    "+6282230109923",
		"+14155552671":      "+14155552671",
	}
	for in, want := range ok {
		got, valid := NormalizePhone(in)
		if !valid || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "12345", "0", "+0123456789", "81234567890", "+62812345678901234", "abc"} {
		if got, valid := NormalizePhone(in); valid {
			t.Errorf("NormalizePhone(%q) = %q, want invalid", in, got)
		}
	}
}

func TestPromoStatus(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		now  time.Time
		want string
	}{
		{start.Add(-time.Second), PromoUpcoming},
		{start, PromoActive},
		{end.Add(-time.Second), PromoActive},
		{end, PromoEnded}, // endsAt is exclusive
		{end.Add(time.Hour), PromoEnded},
	}
	for _, c := range cases {
		if got := PromoStatus(c.now, start, end); got != c.want {
			t.Errorf("PromoStatus(%v) = %s, want %s", c.now, got, c.want)
		}
	}
	if !PromoPubliclyVisible(end.Add(7*24*time.Hour-time.Second), end) {
		t.Error("ended promo must stay visible for 7 days")
	}
	if PromoPubliclyVisible(end.Add(7*24*time.Hour), end) {
		t.Error("ended promo must be hidden after 7 days")
	}
}

func TestReadMinutes(t *testing.T) {
	if got := ReadMinutes(""); got != 1 {
		t.Errorf("empty = %d", got)
	}
	if got := ReadMinutes("<p>" + strings.Repeat("kata ", 200) + "</p>"); got != 1 {
		t.Errorf("200 words = %d", got)
	}
	if got := ReadMinutes("<p>" + strings.Repeat("kata ", 201) + "</p><ul><li>x</li></ul>"); got != 2 {
		t.Errorf("202 words = %d", got)
	}
	if got := PlainText("<p>a</p><p>b&amp;c</p>"); strings.Join(strings.Fields(got), " ") != "a b&c" {
		t.Errorf("PlainText = %q", got)
	}
}

func TestSanitizeHTML(t *testing.T) {
	out := SanitizeHTML(`<p onclick="x()">Hi <a href="javascript:alert(1)">x</a><a href="https://e.com">y</a></p><script>bad()</script><iframe src="x"></iframe><img src="/uploads/a.png">`)
	for _, bad := range []string{"onclick", "javascript:", "<script", "<iframe", "bad()"} {
		if strings.Contains(out, bad) {
			t.Errorf("sanitised output contains %q: %s", bad, out)
		}
	}
	for _, good := range []string{`href="https://e.com"`, `<img src="/uploads/a.png"`} {
		if !strings.Contains(out, good) {
			t.Errorf("sanitised output lost %q: %s", good, out)
		}
	}
}

func TestValidator(t *testing.T) {
	v := NewValidator()
	v.Str("title", "", true, 10)
	v.Str("title", "x", true, 10) // first error wins
	v.Str("eyebrow", "ééééééééééé", false, 10)
	v.StrList("points", []string{"ok", ""}, 10, 5)
	v.MaxItems("tags", 11, 10)
	v.Email("email", "a@b")
	u := "javascript:alert(1)"
	v.HTTPURL("mapsUrl", &u)
	m := "/uploads/../x"
	v.MediaURL("seo.ogImageUrl", &m, false)
	v.Enum("status", "archived", "draft", "published")
	err := v.Err()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	want := map[string]string{"title": MsgRequired, "eyebrow": MaxLenMsg(10), "points.1": MsgRequired,
		"tags": "Maksimal 10 item.", "email": MsgEmail, "mapsUrl": MsgHTTPURL, "seo.ogImageUrl": MsgMediaURL, "status": MsgInvalid}
	for k, msg := range want {
		if ve.Fields[k] != msg {
			t.Errorf("field %s = %q, want %q", k, ve.Fields[k], msg)
		}
	}
	if NewValidator().Err() != nil {
		t.Error("empty validator must return nil")
	}
	ok := "/uploads/0123abcd.webp"
	v2 := NewValidator()
	v2.MediaURL("x", &ok, true)
	h := "https://example.com/a.png"
	v2.MediaURL("y", &h, true)
	v2.HTTPURL("z", &h)
	if v2.Err() != nil {
		t.Errorf("valid URLs rejected: %v", v2.Fields)
	}
}

func TestTrimHelpers(t *testing.T) {
	empty := "   "
	if TrimPtr(&empty) != nil {
		t.Error(`"" must become nil`)
	}
	s := " a "
	if *TrimPtr(&s) != "a" {
		t.Error("trim")
	}
	if l := TrimList(nil); l == nil || len(l) != 0 {
		t.Error("TrimList(nil) must be empty, non-nil")
	}
}

func TestStripHTML(t *testing.T) {
	cases := map[string]string{
		"Berapa lama?":                         "Berapa lama?",
		"<b>Tebal</b> & <i>miring</i>":         "Tebal & miring",
		"HRIS &amp; DMS":                       "HRIS & DMS",
		"a < b dan c > d":                      "a < b dan c > d",
		`<script>alert(1)</script>Aman`:        "Aman",
		`<a href="javascript:x()">tautan</a>`:  "tautan",
		"&lt;img src=x onerror=alert(1)&gt;ok": "ok",
		"<p></p>":                              "",
	}
	for in, want := range cases {
		got := StripHTML(in)
		if got != want {
			t.Errorf("StripHTML(%q) = %q, want %q", in, got, want)
		}
		if again := StripHTML(got); again != got {
			t.Errorf("StripHTML not stable for %q: %q → %q", in, got, again)
		}
	}
}
