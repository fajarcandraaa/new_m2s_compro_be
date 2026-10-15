package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const userCols = `id, name, email, password_hash, role, is_active, session_version, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var v domain.User
	err := row.Scan(&v.ID, &v.Name, &v.Email, &v.PasswordHash, &v.Role, &v.IsActive, &v.SessionVersion,
		&v.LastLoginAt, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// GetUser returns a user by id (implements auth.UserLoader).
func (s *Store) GetUser(ctx context.Context, id string) (*domain.User, error) {
	v, err := scanUser(s.db.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get user: %w", err)
	}
	return &v, nil
}

// GetUserByEmail looks a user up case-insensitively.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	v, err := scanUser(s.db.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE lower(email) = lower($1)", email))
	if err != nil {
		return nil, fmt.Errorf("store: get user by email: %w", err)
	}
	return &v, nil
}

// TouchLogin sets last_login_at = now().
func (s *Store) TouchLogin(ctx context.Context, id string) error {
	if _, err := s.db.Exec(ctx, "UPDATE users SET last_login_at = now() WHERE id = $1", id); err != nil {
		return fmt.Errorf("store: touch login: %w", err)
	}
	return nil
}

// ListUsers returns users ordered by name.
func (s *Store) ListUsers(ctx context.Context, q string, p Page) ([]domain.User, int, error) {
	var w where
	if q != "" {
		w.add("(name ILIKE $? OR email ILIKE $?)", likePattern(q))
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM users"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count users: %w", err)
	}
	limit := w.next(p.Size)
	offset := w.next(p.offset())
	rows, err := s.db.Query(ctx, "SELECT "+userCols+" FROM users"+w.sql()+" ORDER BY lower(name), id LIMIT "+limit+" OFFSET "+offset, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list users: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.User, error) { return scanUser(r) })
	if err != nil {
		return nil, 0, fmt.Errorf("store: list users: %w", err)
	}
	return list, total, nil
}

// EmailTaken reports whether another user has email (case-insensitive).
func (s *Store) EmailTaken(ctx context.Context, email, exceptID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1) AND id::text <> $2)", email, exceptID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: email taken: %w", err)
	}
	return ok, nil
}

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, name, email, role, hash string) (*domain.User, error) {
	v, err := scanUser(s.db.QueryRow(ctx, `INSERT INTO users (name, email, role, password_hash)
		VALUES ($1, $2, $3, $4) RETURNING `+userCols, name, email, role, hash))
	if err != nil {
		return nil, fmt.Errorf("store: create user: %w", err)
	}
	return &v, nil
}

// UpdateUser fully updates a user; bumpSession increments session_version.
func (s *Store) UpdateUser(ctx context.Context, id, name, email, role string, active, bumpSession bool) (*domain.User, error) {
	v, err := scanUser(s.db.QueryRow(ctx, `UPDATE users SET name = $2, email = $3, role = $4, is_active = $5,
		session_version = session_version + CASE WHEN $6 THEN 1 ELSE 0 END, updated_at = now()
		WHERE id = $1 RETURNING `+userCols, id, name, email, role, active, bumpSession))
	if err != nil {
		return nil, fmt.Errorf("store: update user: %w", err)
	}
	return &v, nil
}

// SetPassword sets a new hash and increments session_version.
func (s *Store) SetPassword(ctx context.Context, id, hash string) (*domain.User, error) {
	v, err := scanUser(s.db.QueryRow(ctx, `UPDATE users SET password_hash = $2,
		session_version = session_version + 1, updated_at = now() WHERE id = $1 RETURNING `+userCols, id, hash))
	if err != nil {
		return nil, fmt.Errorf("store: set password: %w", err)
	}
	return &v, nil
}

// DeleteUser deletes a user.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "users", id)
}
