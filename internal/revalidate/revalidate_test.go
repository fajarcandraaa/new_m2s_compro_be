package revalidate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFire(t *testing.T) {
	got := make(chan []string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/revalidate" || r.Header.Get("X-Revalidate-Secret") != "s3cret" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad request: %s %v", r.URL.Path, r.Header)
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got <- body.Tags
	}))
	defer srv.Close()
	New(srv.URL, "s3cret").Fire(TagServices, TagPromos)
	select {
	case tags := <-got:
		if len(tags) != 2 || tags[0] != "services" || tags[1] != "promos" {
			t.Fatalf("tags: %v", tags)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revalidate not sent")
	}
	// Disabled without a secret / URL: must not panic or block.
	New("", "x").Fire(TagSite)
	New(srv.URL, "").Fire(TagSite)
	// Unreachable FE: logged, never blocks the caller.
	start := time.Now()
	New("http://127.0.0.1:1", "x").Fire(TagSite)
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Fire blocked")
	}
}
