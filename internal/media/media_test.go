package media

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

// 1x1 lossless WebP.
const webp1x1 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func sample(t *testing.T, kind string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	switch kind {
	case "png":
		_ = png.Encode(&buf, img)
	case "jpeg":
		_ = jpeg.Encode(&buf, img, nil)
	case "webp":
		b, _ := base64.StdEncoding.DecodeString(webp1x1)
		return b
	}
	return buf.Bytes()
}

func TestDetect(t *testing.T) {
	for kind, want := range map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"} {
		mime, _, w, h, err := Detect(sample(t, kind))
		if err != nil || mime != want || w < 1 || h < 1 {
			t.Errorf("%s: %s %dx%d %v", kind, mime, w, h, err)
		}
	}
	for name, data := range map[string][]byte{
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"text": []byte("hello"),
		"gif":  []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"),
		"fake": append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...),
	} {
		var he *httpx.Error
		if _, _, _, _, err := Detect(data); !errors.As(err, &he) || he.Status != http.StatusUnsupportedMediaType {
			t.Errorf("%s: want 415, got %v", name, err)
		}
	}
}

func TestStoreAndServe(t *testing.T) {
	dir := t.TempDir()
	up, err := Store(sample(t, "png"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Filename) != 36 || filepath.Ext(up.Filename) != ".png" || up.URL != "/uploads/"+up.Filename {
		t.Fatalf("upload: %+v", up)
	}
	st, err := os.Stat(filepath.Join(dir, up.Filename))
	if err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("file mode: %v %v", st, err)
	}
	h := FileServer(dir)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", up.URL, nil))
	if rec.Code != 200 || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("serve: %d %v", rec.Code, rec.Header())
	}
	for _, p := range []string{"/uploads/", "/uploads/../x", "/uploads/.upload-1", "/uploads/nope.png"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != 404 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
