package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const testimonialCols = `id, client_name, client_role, company, quote, is_active, sort_order, created_at, updated_at`

func scanTestimonial(row pgx.Row) (domain.Testimonial, error) {
	var v domain.Testimonial
	err := row.Scan(&v.ID, &v.ClientName, &v.ClientRole, &v.Company, &v.Quote, &v.IsActive, &v.SortOrder,
		&v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// ListTestimonials returns testimonials by sort_order. active filters on
// is_active; limit 0 = all.
func (s *Store) ListTestimonials(ctx context.Context, active *bool, limit int) ([]domain.Testimonial, error) {
	var w where
	if active != nil {
		w.add("is_active = $?", *active)
	}
	q := "SELECT " + testimonialCols + " FROM testimonials" + w.sql() + " ORDER BY sort_order, created_at"
	if limit > 0 {
		q += " LIMIT " + w.next(limit)
	}
	rows, err := s.db.Query(ctx, q, w.args...)
	if err != nil {
		return nil, fmt.Errorf("store: list testimonials: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Testimonial, error) { return scanTestimonial(r) })
	if err != nil {
		return nil, fmt.Errorf("store: list testimonials: %w", err)
	}
	return list, nil
}

// GetTestimonial returns a testimonial by id.
func (s *Store) GetTestimonial(ctx context.Context, id string) (*domain.Testimonial, error) {
	v, err := scanTestimonial(s.db.QueryRow(ctx, "SELECT "+testimonialCols+" FROM testimonials WHERE id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get testimonial: %w", err)
	}
	return &v, nil
}

// SaveTestimonial creates (appended) or fully updates a testimonial.
func (s *Store) SaveTestimonial(ctx context.Context, id string, v *domain.Testimonial) (*domain.Testimonial, error) {
	if id == "" {
		err := s.Tx(ctx, func(tx *Store) error {
			if _, err := tx.db.Exec(ctx, "LOCK TABLE testimonials IN SHARE ROW EXCLUSIVE MODE"); err != nil {
				return fmt.Errorf("store: lock testimonials: %w", err)
			}
			order, err := tx.nextSortOrder(ctx, "testimonials")
			if err != nil {
				return err
			}
			return tx.db.QueryRow(ctx, `INSERT INTO testimonials (client_name, client_role, company, quote, is_active, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text`,
				v.ClientName, v.ClientRole, v.Company, v.Quote, v.IsActive, order).Scan(&id)
		})
		if err != nil {
			return nil, fmt.Errorf("store: insert testimonial: %w", err)
		}
		return s.GetTestimonial(ctx, id)
	}
	tag, err := s.db.Exec(ctx, `UPDATE testimonials SET client_name = $1, client_role = $2, company = $3,
		quote = $4, is_active = $5, updated_at = now() WHERE id = $6`,
		v.ClientName, v.ClientRole, v.Company, v.Quote, v.IsActive, id)
	if err != nil {
		return nil, fmt.Errorf("store: update testimonial: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("store: update testimonial: %w", pgx.ErrNoRows)
	}
	return s.GetTestimonial(ctx, id)
}

// DeleteTestimonial deletes a testimonial.
func (s *Store) DeleteTestimonial(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "testimonials", id)
}

// ReorderTestimonials sets sort_order from ids.
func (s *Store) ReorderTestimonials(ctx context.Context, ids []string) error {
	return s.reorder(ctx, "testimonials", ids)
}

const clientLogoCols = `id, name, image_url, sort_order, created_at, updated_at`

func scanClientLogo(row pgx.Row) (domain.ClientLogo, error) {
	var v domain.ClientLogo
	err := row.Scan(&v.ID, &v.Name, &v.ImageURL, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// ListClientLogos returns all logos by sort_order.
func (s *Store) ListClientLogos(ctx context.Context) ([]domain.ClientLogo, error) {
	rows, err := s.db.Query(ctx, "SELECT "+clientLogoCols+" FROM client_logos ORDER BY sort_order, created_at")
	if err != nil {
		return nil, fmt.Errorf("store: list client logos: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ClientLogo, error) { return scanClientLogo(r) })
	if err != nil {
		return nil, fmt.Errorf("store: list client logos: %w", err)
	}
	return list, nil
}

// GetClientLogo returns a logo by id.
func (s *Store) GetClientLogo(ctx context.Context, id string) (*domain.ClientLogo, error) {
	v, err := scanClientLogo(s.db.QueryRow(ctx, "SELECT "+clientLogoCols+" FROM client_logos WHERE id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get client logo: %w", err)
	}
	return &v, nil
}

// SaveClientLogo creates (appended) or fully updates a logo.
func (s *Store) SaveClientLogo(ctx context.Context, id string, v *domain.ClientLogo) (*domain.ClientLogo, error) {
	if id == "" {
		err := s.Tx(ctx, func(tx *Store) error {
			if _, err := tx.db.Exec(ctx, "LOCK TABLE client_logos IN SHARE ROW EXCLUSIVE MODE"); err != nil {
				return fmt.Errorf("store: lock client logos: %w", err)
			}
			order, err := tx.nextSortOrder(ctx, "client_logos")
			if err != nil {
				return err
			}
			return tx.db.QueryRow(ctx, `INSERT INTO client_logos (name, image_url, sort_order)
				VALUES ($1, $2, $3) RETURNING id::text`, v.Name, v.ImageURL, order).Scan(&id)
		})
		if err != nil {
			return nil, fmt.Errorf("store: insert client logo: %w", err)
		}
		return s.GetClientLogo(ctx, id)
	}
	tag, err := s.db.Exec(ctx, `UPDATE client_logos SET name = $1, image_url = $2, updated_at = now() WHERE id = $3`,
		v.Name, v.ImageURL, id)
	if err != nil {
		return nil, fmt.Errorf("store: update client logo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("store: update client logo: %w", pgx.ErrNoRows)
	}
	return s.GetClientLogo(ctx, id)
}

// DeleteClientLogo deletes a logo row (the file is kept).
func (s *Store) DeleteClientLogo(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "client_logos", id)
}

// ReorderClientLogos sets sort_order from ids.
func (s *Store) ReorderClientLogos(ctx context.Context, ids []string) error {
	return s.reorder(ctx, "client_logos", ids)
}
