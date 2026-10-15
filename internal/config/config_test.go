package config

import (
	"log/slog"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "JWT_SECRET": "0123456789abcdef0123456789abcdef"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || !c.MigrateOnStart || !c.SeedOnStart || c.JWTTTL != 12*time.Hour || !c.CookieSecure ||
		c.AdminName != "Admin" || c.UploadDir != "/data/uploads" || c.UploadMaxBytes != 2097152 ||
		c.FrontendURL != "http://fe:3000" || c.TrustedProxyHops != 1 || c.LogLevel != slog.LevelInfo {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(env(map[string]string{})); err == nil {
		t.Fatal("missing DATABASE_URL/JWT_SECRET must fail")
	}
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "JWT_SECRET": "short"})); err == nil {
		t.Fatal("short JWT_SECRET must fail")
	}
	base := map[string]string{"DATABASE_URL": "x", "JWT_SECRET": "0123456789abcdef0123456789abcdef"}
	for k, v := range map[string]string{"JWT_TTL": "soon", "COOKIE_SECURE": "maybe", "TRUSTED_PROXY_HOPS": "-1", "LOG_LEVEL": "loud", "UPLOAD_MAX_BYTES": "0"} {
		m := map[string]string{k: v}
		for bk, bv := range base {
			m[bk] = bv
		}
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%s=%s must fail", k, v)
		}
	}
}
