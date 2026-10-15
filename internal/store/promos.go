package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

// promoStatusSQL computes the status at query time (never stale).
const promoStatusSQL = `(CASE WHEN now() < p.starts_at THEN 'upcoming' WHEN now() < p.ends_at THEN 'active' ELSE 'ended' END)`

const promoCols = `p.id, p.title, p.description, p.code, p.starts_at, p.ends_at, ` + promoStatusSQL + `,
	p.service_id::text, s.slug, s.title, p.created_at, p.updated_at`

func scanPromo(row pgx.Row) (domain.Promo, error) {
	var v domain.Promo
	var sSlug, sTitle *string
	err := row.Scan(&v.ID, &v.Title, &v.Description, &v.Code, &v.StartsAt, &v.EndsAt, &v.Status,
		&v.ServiceID, &sSlug, &sTitle, &v.CreatedAt, &v.UpdatedAt)
	if sSlug != nil && sTitle != nil {
		v.Service = &domain.ServiceRef{Slug: *sSlug, Title: *sTitle}
	}
	return v, err
}

func collectPromos(rows pgx.Rows) ([]domain.Promo, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Promo, error) { return scanPromo(r) })
}

// ListPublicPromos returns promos visible on the public site: active,
// upcoming and ended within the last 7 days. The embedded service is only
// shown when it is published. Order: active (ends asc), upcoming (starts
// asc), ended (ends desc).
func (s *Store) ListPublicPromos(ctx context.Context, status string) ([]domain.Promo, error) {
	var w where
	w.add("p.ends_at > now() - interval '7 days'")
	if status != "" {
		w.add(promoStatusSQL+" = $?", status)
	}
	rows, err := s.db.Query(ctx, "SELECT "+promoCols+` FROM promos p
		LEFT JOIN services s ON s.id = p.service_id AND s.status = 'published'`+w.sql()+`
		ORDER BY CASE `+promoStatusSQL+` WHEN 'active' THEN 0 WHEN 'upcoming' THEN 1 ELSE 2 END,
		CASE WHEN `+promoStatusSQL+` = 'active' THEN p.ends_at END ASC,
		CASE WHEN `+promoStatusSQL+` = 'upcoming' THEN p.starts_at END ASC,
		CASE WHEN `+promoStatusSQL+` = 'ended' THEN p.ends_at END DESC, p.id`, w.args...)
	if err != nil {
		return nil, fmt.Errorf("store: list promos: %w", err)
	}
	list, err := collectPromos(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list promos: %w", err)
	}
	return list, nil
}

// AdminPromoFilter filters the CMS promo table.
type AdminPromoFilter struct {
	Q      string
	Status string
	Page   Page
}

// ListAdminPromos returns all promos (incl. every ended one) by updated_at desc.
func (s *Store) ListAdminPromos(ctx context.Context, f AdminPromoFilter) ([]domain.Promo, int, error) {
	var w where
	if f.Q != "" {
		w.add("(p.title ILIKE $? OR p.code ILIKE $?)", likePattern(f.Q))
	}
	if f.Status != "" {
		w.add(promoStatusSQL+" = $?", f.Status)
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM promos p"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count promos: %w", err)
	}
	limit := w.next(f.Page.Size)
	offset := w.next(f.Page.offset())
	rows, err := s.db.Query(ctx, "SELECT "+promoCols+" FROM promos p LEFT JOIN services s ON s.id = p.service_id"+
		w.sql()+" ORDER BY p.updated_at DESC, p.id LIMIT "+limit+" OFFSET "+offset, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list promos: %w", err)
	}
	list, err := collectPromos(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list promos: %w", err)
	}
	return list, total, nil
}

// GetPromo returns a promo by id with its service regardless of status.
func (s *Store) GetPromo(ctx context.Context, id string) (*domain.Promo, error) {
	v, err := scanPromo(s.db.QueryRow(ctx, "SELECT "+promoCols+" FROM promos p LEFT JOIN services s ON s.id = p.service_id WHERE p.id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get promo: %w", err)
	}
	return &v, nil
}

// SavePromo creates (id == "") or fully updates a promo.
func (s *Store) SavePromo(ctx context.Context, id string, v *domain.Promo) (*domain.Promo, error) {
	args := []any{v.Title, v.Description, v.Code, v.StartsAt, v.EndsAt, v.ServiceID}
	if id == "" {
		if err := s.db.QueryRow(ctx, `INSERT INTO promos (title, description, code, starts_at, ends_at, service_id)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text`, args...).Scan(&id); err != nil {
			return nil, fmt.Errorf("store: insert promo: %w", err)
		}
	} else {
		args = append(args, id)
		tag, err := s.db.Exec(ctx, `UPDATE promos SET title = $1, description = $2, code = $3,
			starts_at = $4, ends_at = $5, service_id = $6, updated_at = now() WHERE id = $7`, args...)
		if err != nil {
			return nil, fmt.Errorf("store: update promo: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("store: update promo: %w", pgx.ErrNoRows)
		}
	}
	return s.GetPromo(ctx, id)
}

// DeletePromo deletes a promo.
func (s *Store) DeletePromo(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "promos", id)
}
