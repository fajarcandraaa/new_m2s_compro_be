// Package media handles image uploads (content-sniffed JPEG/PNG/WebP,
// random names) and serves the upload directory.
package media

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

// Upload is the stored file's metadata.
type Upload struct {
	URL      string
	Filename string
	MimeType string
	Size     int
	Width    int
	Height   int
}

var allowed = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// multipartOverhead is the extra request size allowed beyond the file.
const multipartOverhead = 64 << 10

// Save reads the `file` part of a multipart request, validates it and
// writes it to dir under a random name.
func Save(w http.ResponseWriter, r *http.Request, dir string, maxBytes int64) (*Upload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, httpx.BadRequest("Body harus multipart/form-data dengan bagian `file`.")
	}
	var data []byte
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, mapReadErr(err)
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		data, err = readLimited(part, maxBytes)
		_ = part.Close()
		if err != nil {
			return nil, err
		}
		break
	}
	if data == nil {
		return nil, httpx.BadRequest("Bagian `file` wajib diisi.")
	}
	return Store(data, dir)
}

func readLimited(part *multipart.Part, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		return nil, mapReadErr(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, httpx.PayloadTooLarge()
	}
	if len(data) == 0 {
		return nil, httpx.BadRequest("File kosong.")
	}
	return data, nil
}

func mapReadErr(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return httpx.PayloadTooLarge()
	}
	return httpx.BadRequest("Body multipart tidak valid.")
}

// Detect sniffs the content type and decodes the image dimensions. Only
// JPEG, PNG and WebP that actually decode are accepted (else 415).
func Detect(data []byte) (mime, ext string, width, height int, err error) {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mime = http.DetectContentType(head)
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	ext, ok := allowed[mime]
	if !ok {
		return "", "", 0, 0, httpx.UnsupportedMediaType()
	}
	cfg, format, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil || cfg.Width < 1 || cfg.Height < 1 || "image/"+format != mime {
		return "", "", 0, 0, httpx.UnsupportedMediaType()
	}
	return mime, ext, cfg.Width, cfg.Height, nil
}

// Store validates data and writes it atomically (temp file + rename).
func Store(data []byte, dir string) (*Upload, error) {
	mime, ext, width, height, err := Detect(data)
	if err != nil {
		return nil, err
	}
	var rnd [16]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return nil, fmt.Errorf("media: random name: %w", err)
	}
	name := hex.EncodeToString(rnd[:]) + ext
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("media: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("media: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("media: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("media: close: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return nil, fmt.Errorf("media: chmod: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return nil, fmt.Errorf("media: rename: %w", err)
	}
	return &Upload{URL: "/uploads/" + name, Filename: name, MimeType: mime, Size: len(data), Width: width, Height: height}, nil
}

// FileServer serves dir at /uploads/ without directory listings, with
// nosniff and immutable caching (names are random).
func FileServer(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/uploads/")
		if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\") {
			httpx.JSONNotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			httpx.JSONNotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			httpx.JSONNotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, name, st.ModTime(), f)
	})
}
