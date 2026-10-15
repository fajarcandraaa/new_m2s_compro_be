package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const serviceCols = `id, slug, audience, status, sort_order, eyebrow, title, tagline, summary,
	price_label, price_value, audience_text, points, deliverables, systems, faqs, seo_title,
	seo_description, og_image_url, created_at, updated_at`

func scanService(row pgx.Row) (domain.Service, error) {
	var v domain.Service
	err := row.Scan(&v.ID, &v.Slug, &v.Audience, &v.Status, &v.SortOrder, &v.Eyebrow, &v.Title,
		&v.Tagline, &v.Summary, &v.PriceLabel, &v.PriceValue, &v.AudienceText, &v.Points,
		&v.Deliverables, &v.Systems, &v.Faqs, &v.Seo.Title, &v.Seo.Description, &v.Seo.OgImageURL,
		&v.CreatedAt, &v.UpdatedAt)
	v.Points = nonNil(v.Points)
	v.Deliverables = nonNil(v.Deliverables)
	v.Systems = nonNil(v.Systems)
	v.Faqs = nonNil(v.Faqs)
	v.Packages = []domain.ServicePackage{}
	return v, err
}

// ServiceFilter filters service lists.
type ServiceFilter struct {
	PublishedOnly bool
	Status        string
	Audience      string
}

// ListServices returns services ordered by sort_order with their packages.
func (s *Store) ListServices(ctx context.Context, f ServiceFilter) ([]domain.Service, error) {
	var w where
	if f.PublishedOnly {
		w.add("status = 'published'")
	}
	if f.Status != "" {
		w.add("status = $?", f.Status)
	}
	if f.Audience != "" {
		w.add("audience = $?", f.Audience)
	}
	rows, err := s.db.Query(ctx, "SELECT "+serviceCols+" FROM services"+w.sql()+" ORDER BY sort_order, created_at", w.args...)
	if err != nil {
		return nil, fmt.Errorf("store: list services: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Service, error) { return scanService(r) })
	if err != nil {
		return nil, fmt.Errorf("store: list services: %w", err)
	}
	if err := s.attachPackages(ctx, list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) attachPackages(ctx context.Context, list []domain.Service) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, len(list))
	idx := make(map[string]int, len(list))
	for i, v := range list {
		ids[i] = v.ID
		idx[v.ID] = i
	}
	rows, err := s.db.Query(ctx, `SELECT service_id::text, name, price_label, price_value, description,
		features, highlighted, sort_order FROM service_packages WHERE service_id = ANY($1::uuid[])
		ORDER BY service_id, sort_order`, ids)
	if err != nil {
		return fmt.Errorf("store: packages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid string
		var p domain.ServicePackage
		if err := rows.Scan(&sid, &p.Name, &p.PriceLabel, &p.PriceValue, &p.Description, &p.Features,
			&p.Highlighted, &p.SortOrder); err != nil {
			return fmt.Errorf("store: packages scan: %w", err)
		}
		p.Features = nonNil(p.Features)
		i := idx[sid]
		list[i].Packages = append(list[i].Packages, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: packages rows: %w", err)
	}
	return nil
}

func (s *Store) getService(ctx context.Context, cond string, arg any) (*domain.Service, error) {
	v, err := scanService(s.db.QueryRow(ctx, "SELECT "+serviceCols+" FROM services WHERE "+cond, arg))
	if err != nil {
		return nil, fmt.Errorf("store: get service: %w", err)
	}
	list := []domain.Service{v}
	if err := s.attachPackages(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// GetService returns a service by id (any status).
func (s *Store) GetService(ctx context.Context, id string) (*domain.Service, error) {
	return s.getService(ctx, "id = $1", id)
}

// GetPublishedServiceBySlug returns a published service by slug.
func (s *Store) GetPublishedServiceBySlug(ctx context.Context, slug string) (*domain.Service, error) {
	return s.getService(ctx, "slug = $1 AND status = 'published'", slug)
}

// PublishedServiceExists reports whether a published service has slug.
func (s *Store) PublishedServiceExists(ctx context.Context, slug string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM services WHERE slug = $1 AND status = 'published')", slug).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: service exists: %w", err)
	}
	return ok, nil
}

// ServiceExists reports whether a service with id exists.
func (s *Store) ServiceExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM services WHERE id = $1)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("store: service exists: %w", err)
	}
	return ok, nil
}

// SaveService creates (id == "") or fully updates a service and replaces
// its packages. v.Slug == "" means generate (create) or keep (update). When
// keepSortOrder is false on create the service is appended.
func (s *Store) SaveService(ctx context.Context, id string, v *domain.Service) (*domain.Service, error) {
	var savedID string
	err := s.Tx(ctx, func(tx *Store) error {
		slug, err := tx.resolveSlug(ctx, "services", id, v.Slug, v.Title)
		if err != nil {
			return err
		}
		if id == "" {
			order := v.SortOrder
			if order < 1 {
				if order, err = tx.nextSortOrder(ctx, "services"); err != nil {
					return err
				}
			}
			err = tx.db.QueryRow(ctx, `INSERT INTO services (slug, audience, status, sort_order, eyebrow,
				title, tagline, summary, price_label, price_value, audience_text, points, deliverables,
				systems, seo_title, seo_description, og_image_url, faqs)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
				RETURNING id::text`,
				slug, v.Audience, v.Status, order, v.Eyebrow, v.Title, v.Tagline, v.Summary, v.PriceLabel,
				v.PriceValue, v.AudienceText, nonNil(v.Points), nonNil(v.Deliverables), nonNil(v.Systems),
				v.Seo.Title, v.Seo.Description, v.Seo.OgImageURL, nonNil(v.Faqs)).Scan(&savedID)
			if err != nil {
				return fmt.Errorf("store: insert service: %w", err)
			}
		} else {
			tag, err := tx.db.Exec(ctx, `UPDATE services SET slug = $2, audience = $3, status = $4,
				eyebrow = $5, title = $6, tagline = $7, summary = $8, price_label = $9, price_value = $10,
				audience_text = $11, points = $12, deliverables = $13, systems = $14, seo_title = $15,
				seo_description = $16, og_image_url = $17, faqs = $18, updated_at = now() WHERE id = $1`,
				id, slug, v.Audience, v.Status, v.Eyebrow, v.Title, v.Tagline, v.Summary, v.PriceLabel,
				v.PriceValue, v.AudienceText, nonNil(v.Points), nonNil(v.Deliverables), nonNil(v.Systems),
				v.Seo.Title, v.Seo.Description, v.Seo.OgImageURL, nonNil(v.Faqs))
			if err != nil {
				return fmt.Errorf("store: update service: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("store: update service: %w", pgx.ErrNoRows)
			}
			savedID = id
			if _, err := tx.db.Exec(ctx, "DELETE FROM service_packages WHERE service_id = $1", id); err != nil {
				return fmt.Errorf("store: delete packages: %w", err)
			}
		}
		for i, p := range v.Packages {
			_, err := tx.db.Exec(ctx, `INSERT INTO service_packages (service_id, name, price_label,
				price_value, description, features, highlighted, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				savedID, p.Name, p.PriceLabel, p.PriceValue, p.Description, nonNil(p.Features), p.Highlighted, i+1)
			if err != nil {
				return fmt.Errorf("store: insert package: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetService(ctx, savedID)
}

// SetServiceFaqsIfEmpty sets the FAQs of the service with slug only while
// its faqs are still `[]`, so admin edits are never overwritten (seed).
// It reports whether a row was updated.
func (s *Store) SetServiceFaqsIfEmpty(ctx context.Context, slug string, faqs []domain.Faq) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE services SET faqs = $2, updated_at = now()
		WHERE slug = $1 AND faqs = '[]'::jsonb`, slug, nonNil(faqs))
	if err != nil {
		return false, fmt.Errorf("store: set service faqs: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteService deletes a service (packages cascade, promos set null).
func (s *Store) DeleteService(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "services", id)
}

// ReorderServices sets sort_order from ids.
func (s *Store) ReorderServices(ctx context.Context, ids []string) error {
	return s.reorder(ctx, "services", ids)
}
