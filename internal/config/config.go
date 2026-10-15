// Package config parses and validates the backend configuration from the
// environment (see docs/architecture.md §8).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Config is the validated runtime configuration.
type Config struct {
	Port             string
	DatabaseURL      string
	MigrateOnStart   bool
	SeedOnStart      bool
	JWTSecret        string
	JWTTTL           time.Duration
	CookieSecure     bool
	AdminEmail       string
	AdminPassword    string
	AdminName        string
	UploadDir        string
	UploadMaxBytes   int64
	FrontendURL      string
	RevalidateSecret string
	TrustedProxyHops int
	LogLevel         slog.Level
}

// Load reads the configuration through getenv (os.Getenv in production) and
// fails fast on missing or invalid values.
func Load(getenv func(string) string) (*Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	boolean := func(key string, def bool) bool {
		raw := get(key, "")
		if raw == "" {
			return def
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid boolean %q", key, raw))
			return def
		}
		return b
	}

	c := &Config{
		Port:             get("PORT", "8080"),
		DatabaseURL:      get("DATABASE_URL", ""),
		MigrateOnStart:   boolean("MIGRATE_ON_START", true),
		SeedOnStart:      boolean("SEED_ON_START", true),
		JWTSecret:        getenv("JWT_SECRET"),
		CookieSecure:     boolean("COOKIE_SECURE", true),
		AdminEmail:       get("ADMIN_EMAIL", ""),
		AdminPassword:    getenv("ADMIN_PASSWORD"),
		AdminName:        get("ADMIN_NAME", "Admin"),
		UploadDir:        get("UPLOAD_DIR", "/data/uploads"),
		FrontendURL:      strings.TrimRight(get("FRONTEND_URL", "http://fe:3000"), "/"),
		RevalidateSecret: getenv("REVALIDATE_SECRET"),
	}

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET is required and must be at least 32 characters"))
	}

	ttl, err := time.ParseDuration(get("JWT_TTL", "12h"))
	if err != nil || ttl <= 0 {
		errs = append(errs, fmt.Errorf("JWT_TTL: invalid duration %q", get("JWT_TTL", "")))
	}
	c.JWTTTL = ttl

	maxBytes, err := strconv.ParseInt(get("UPLOAD_MAX_BYTES", "2097152"), 10, 64)
	if err != nil || maxBytes <= 0 {
		errs = append(errs, errors.New("UPLOAD_MAX_BYTES must be a positive integer"))
	}
	c.UploadMaxBytes = maxBytes

	hops, err := strconv.Atoi(get("TRUSTED_PROXY_HOPS", "1"))
	if err != nil || hops < 0 {
		errs = append(errs, errors.New("TRUSTED_PROXY_HOPS must be an integer >= 0"))
	}
	c.TrustedProxyHops = hops

	switch strings.ToLower(get("LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL: unknown level %q", get("LOG_LEVEL", "")))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return c, nil
}
