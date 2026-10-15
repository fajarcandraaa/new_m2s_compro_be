// Package store contains the hand-written SQL per resource (pgx, no ORM).
package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

// DB is satisfied by *pgxpool.Pool and pgx.Tx.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store runs queries against a pool or a transaction.
type Store struct {
	db DB
}

// New returns a store over db.
func New(db DB) *Store { return &Store{db: db} }

// Tx runs fn inside a transaction (a savepoint when already in one).
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		return fn(&Store{db: tx})
	})
}

// Ping checks the database.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.db.Exec(ctx, "SELECT 1")
	return err
}

// where builds a parameterised WHERE clause. "$?" in a condition is
// replaced with the placeholder of the argument added with it.
type where struct {
	conds []string
	args  []any
}

func (w *where) add(cond string, arg ...any) {
	if len(arg) > 0 {
		w.args = append(w.args, arg[0])
		cond = strings.ReplaceAll(cond, "$?", "$"+strconv.Itoa(len(w.args)))
	}
	w.conds = append(w.conds, cond)
}

func (w *where) sql() string {
	if len(w.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.conds, " AND ")
}

// next returns the placeholder for the next argument and appends it.
func (w *where) next(arg any) string {
	w.args = append(w.args, arg)
	return "$" + strconv.Itoa(len(w.args))
}

// likePattern escapes %, _ and \ and wraps q in %...%.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// Page is a pagination request.
type Page struct {
	Page int
	Size int
}

func (p Page) offset() int { return (p.Page - 1) * p.Size }

// tables with a slug column (whitelist for the slug helpers).
var slugTables = map[string]bool{"services": true, "case_studies": true, "articles": true}

// resolveSlug implements the slug rules:
//   - requested != "": must not be used by another row (409 slug_taken);
//   - update without slug: keep the current slug;
//   - create without slug: slugify(title) with -2, -3, … on collision.
func (s *Store) resolveSlug(ctx context.Context, table, id, requested, title string) (string, error) {
	if !slugTables[table] {
		return "", fmt.Errorf("store: resolveSlug: table %q not allowed", table)
	}
	if requested != "" {
		var taken bool
		err := s.db.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM "+table+" WHERE slug = $1 AND id::text <> $2)", requested, id).Scan(&taken)
		if err != nil {
			return "", fmt.Errorf("store: slug check: %w", err)
		}
		if taken {
			return "", domain.SlugTaken()
		}
		return requested, nil
	}
	if id != "" {
		var current string
		if err := s.db.QueryRow(ctx, "SELECT slug FROM "+table+" WHERE id = $1", id).Scan(&current); err != nil {
			return "", fmt.Errorf("store: current slug: %w", err)
		}
		return current, nil
	}
	base := domain.Slugify(title)
	if base == "" {
		return "", &domain.ValidationError{Message: domain.MsgGeneric, Fields: map[string]string{"title": domain.MsgTitleNoSlug}}
	}
	for n := 1; ; n++ {
		candidate := base
		if n > 1 {
			suffix := "-" + strconv.Itoa(n)
			candidate = domain.TruncateSlug(base, domain.SlugMaxLen-len(suffix)) + suffix
		}
		var taken bool
		if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE slug = $1)", candidate).Scan(&taken); err != nil {
			return "", fmt.Errorf("store: slug check: %w", err)
		}
		if !taken {
			return candidate, nil
		}
	}
}

// tables that can be reordered.
var orderTables = map[string]bool{"services": true, "testimonials": true, "client_logos": true}

// nextSortOrder returns max(sort_order)+1.
func (s *Store) nextSortOrder(ctx context.Context, table string) (int, error) {
	if !orderTables[table] && table != "case_studies" {
		return 0, fmt.Errorf("store: nextSortOrder: table %q not allowed", table)
	}
	var n int
	if err := s.db.QueryRow(ctx, "SELECT COALESCE(MAX(sort_order), 0) + 1 FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: next sort order: %w", err)
	}
	return n, nil
}

// reorder sets sort_order = 1..n following ids, which must contain every
// id of the table exactly once (else 422 on "ids").
func (s *Store) reorder(ctx context.Context, table string, ids []string) error {
	if !orderTables[table] {
		return fmt.Errorf("store: reorder: table %q not allowed", table)
	}
	return s.Tx(ctx, func(tx *Store) error {
		rows, err := tx.db.Query(ctx, "SELECT id::text FROM "+table+" FOR UPDATE")
		if err != nil {
			return fmt.Errorf("store: reorder lock: %w", err)
		}
		existing, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("store: reorder ids: %w", err)
		}
		set := make(map[string]bool, len(existing))
		for _, id := range existing {
			set[id] = true
		}
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			id = strings.ToLower(id)
			if !set[id] || seen[id] {
				return reorderInvalid()
			}
			seen[id] = true
		}
		if len(seen) != len(set) {
			return reorderInvalid()
		}
		_, err = tx.db.Exec(ctx, "UPDATE "+table+" SET sort_order = x.ord, updated_at = now() "+
			"FROM unnest($1::uuid[]) WITH ORDINALITY AS x(id, ord) WHERE "+table+".id = x.id", ids)
		if err != nil {
			return fmt.Errorf("store: reorder update: %w", err)
		}
		return nil
	})
}

func reorderInvalid() error {
	return &domain.ValidationError{Message: domain.MsgGeneric, Fields: map[string]string{
		"ids": "Daftar ID harus berisi setiap item tepat satu kali.",
	}}
}

// deleteByID deletes one row; 0 rows → pgx.ErrNoRows.
func (s *Store) deleteByID(ctx context.Context, table, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("store: delete %s %s: %w", table, id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("store: delete %s %s: %w", table, id, pgx.ErrNoRows)
	}
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
