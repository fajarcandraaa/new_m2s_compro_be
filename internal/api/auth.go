package api

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

const msgBadLogin = "Email atau kata sandi salah."

// login: wrong/malformed credentials or inactive user → uniform 401.
func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	if ok, retry := s.loginLimiter.Allow(httpx.IPFromContext(r.Context())); !ok {
		return httpx.RateLimited(retry)
	}
	var in LoginInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	email := domain.Trim(in.Email)
	if !domain.IsEmail(email) || utf8.RuneCountInString(email) > 254 || in.Password == "" || utf8.RuneCountInString(in.Password) > 64 {
		auth.CheckPassword(nil, in.Password)
		return httpx.Unauthorized(msgBadLogin)
	}
	u, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if u == nil {
		auth.CheckPassword(nil, in.Password)
		return httpx.Unauthorized(msgBadLogin)
	}
	if !auth.CheckPassword(&u.PasswordHash, in.Password) || !u.IsActive {
		return httpx.Unauthorized(msgBadLogin)
	}
	if err := s.store.TouchLogin(r.Context(), u.ID); err != nil {
		return err
	}
	if u, err = s.store.GetUser(r.Context(), u.ID); err != nil {
		return err
	}
	if err := s.auth.SetCookie(w, u); err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toUser(u))
	return nil
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) error {
	s.auth.ClearCookie(w)
	httpx.NoContent(w)
	return nil
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) error {
	httpx.Data(w, http.StatusOK, toUser(auth.UserFrom(r.Context())))
	return nil
}
