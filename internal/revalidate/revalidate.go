// Package revalidate notifies the FE that cached content changed
// (POST {FRONTEND_URL}/api/revalidate, fire-and-forget).
package revalidate

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Cache tags (see info.description of the OpenAPI spec).
const (
	TagSite         = "site"
	TagServices     = "services"
	TagCaseStudies  = "case-studies"
	TagArticles     = "articles"
	TagPromos       = "promos"
	TagTestimonials = "testimonials"
	TagClientLogos  = "client-logos"
)

// Firer fires a revalidation for the given tags without blocking.
type Firer interface {
	Fire(tags ...string)
}

// Client is the HTTP implementation of Firer.
type Client struct {
	URL     string // full endpoint, e.g. http://fe:3000/api/revalidate
	Secret  string
	HTTP    *http.Client
	Timeout time.Duration
}

// New returns a client for frontendURL; an empty URL or secret makes Fire a
// no-op.
func New(frontendURL, secret string) *Client {
	c := &Client{Secret: secret, HTTP: &http.Client{}, Timeout: 3 * time.Second}
	if frontendURL != "" {
		c.URL = frontendURL + "/api/revalidate"
	}
	return c
}

// Fire sends the request in a goroutine with its own timeout, detached from
// the request context. Failures are logged at warn.
func (c *Client) Fire(tags ...string) {
	if c.URL == "" || c.Secret == "" {
		slog.Debug("revalidate disabled", "tags", tags)
		return
	}
	go c.send(tags)
}

func (c *Client) send(tags []string) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	body, _ := json.Marshal(map[string][]string{"tags": tags})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("revalidate request build failed", "err", err.Error(), "tags", tags)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Revalidate-Secret", c.Secret)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		slog.Warn("revalidate failed", "err", err.Error(), "tags", tags)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		slog.Warn("revalidate non-2xx", "status", resp.StatusCode, "tags", tags)
		return
	}
	slog.Debug("revalidated", "tags", tags)
}
