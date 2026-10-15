// Package auth implements password hashing, the JWT session cookie, the
// session middleware, role guard and CSRF header check.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

// CookieName is the session cookie.
const CookieName = "m2s_session"

// CSRFHeader / CSRFValue is the required anti-CSRF header.
const (
	CSRFHeader = "X-Requested-With"
	CSRFValue  = "m2s-cms"
)

// BcryptCost is the bcrypt cost (tests may lower it).
var BcryptCost = 12

// HashPassword hashes a password with bcrypt.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(h), nil
}

// dummyHash keeps login timing flat for unknown emails.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

// CheckPassword compares a password with a hash. A nil hash compares
// against a dummy hash and always fails.
func CheckPassword(hash *string, pw string) bool {
	if hash == nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*hash), []byte(pw)) == nil
}

// PasswordProblem returns a validation message when pw is outside 10–64
// characters or over 72 bytes, else "".
func PasswordProblem(pw string) string {
	n := utf8.RuneCountInString(pw)
	switch {
	case pw == "":
		return domain.MsgRequired
	case n < 10:
		return domain.MinLenMsg(10)
	case n > 64:
		return domain.MaxLenMsg(64)
	case len(pw) > 72:
		return "Maksimal 72 byte."
	}
	return ""
}

// Claims are the JWT claims.
type Claims struct {
	Role string `json:"role"`
	SV   int    `json:"sv"`
	jwt.RegisteredClaims
}

// Manager issues and verifies session tokens.
type Manager struct {
	Secret       []byte
	TTL          time.Duration
	CookieSecure bool
	Users        UserLoader
}

// UserLoader loads a user by id (store).
type UserLoader interface {
	GetUser(ctx context.Context, id string) (*domain.User, error)
}

// Issue creates a signed token for u.
func (m *Manager) Issue(u *domain.User) (string, error) {
	now := time.Now()
	claims := Claims{
		Role: u.Role,
		SV:   u.SessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.TTL)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.Secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign: %w", err)
	}
	return tok, nil
}

// Parse verifies a token and returns its claims.
func (m *Manager) Parse(tok string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (any, error) {
		return m.Secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// SetCookie issues a token for u and sets the session cookie.
func (m *Manager) SetCookie(w http.ResponseWriter, u *domain.User) error {
	tok, err := m.Issue(u)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.TTL.Seconds()),
	})
	return nil
}

// ClearCookie expires the session cookie (Max-Age=0).
func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type ctxKey int

const userKey ctxKey = iota

// UserFrom returns the authenticated user from the context.
func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userKey).(*domain.User)
	return u
}

// WithUser stores u in ctx (tests).
func WithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// RequireSession verifies the cookie, loads the user and checks is_active
// and session_version. A stale session gets 401 and the cookie is cleared.
func (m *Manager) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			httpx.WriteError(w, r, httpx.Unauthorized(""))
			return
		}
		claims, err := m.Parse(c.Value)
		if err != nil || !httpx.IsUUID(claims.Subject) {
			m.ClearCookie(w)
			httpx.WriteError(w, r, httpx.Unauthorized(""))
			return
		}
		u, err := m.Users.GetUser(r.Context(), claims.Subject)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				m.ClearCookie(w)
				httpx.WriteError(w, r, httpx.Unauthorized(""))
				return
			}
			httpx.WriteError(w, r, err)
			return
		}
		if !u.IsActive || u.SessionVersion != claims.SV {
			slog.InfoContext(r.Context(), "stale session rejected", "user_id", u.ID)
			m.ClearCookie(w)
			httpx.WriteError(w, r, httpx.Unauthorized(""))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// RequireRole allows only the given roles (403 forbidden otherwise).
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				httpx.WriteError(w, r, httpx.Unauthorized(""))
				return
			}
			for _, role := range roles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.WriteError(w, r, httpx.Forbidden())
		})
	}
}

// CSRF requires X-Requested-With: m2s-cms on mutating methods.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if r.Header.Get(CSRFHeader) != CSRFValue {
				httpx.WriteError(w, r, httpx.CSRFFailed())
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
