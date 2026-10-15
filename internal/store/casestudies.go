package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const caseStudyCols = `id, slug, status, system_name, category, featured, sort_order, title, summary,
	card_body, metrics, scope, initial_conditions_title, initial_conditions, risks_title, risks,
	approach_title, approach, solutions, impact_title, impacts, lesson, seo_title, seo_description,
	og_image_url, created_at, updated_at`

func scanCaseStudy(row pgx.Row) (domain.CaseStudy, error) {
	var v domain.CaseStudy
	err := row.Scan(&v.ID, &v.Slug, &v.Status, &v.SystemName, &v.Category, &v.Featured, &v.SortOrder,
		&v.Title, &v.Summary, &v.CardBody, &v.Metrics, &v.Scope, &v.InitialConditionsTitle,
		&v.InitialConditions, &v.RisksTitle, &v.Risks, &v.ApproachTitle, &v.Approach, &v.Solutions,
		&v.ImpactTitle, &v.Impacts, &v.Lesson, &v.Seo.Title, &v.Seo.Description, &v.Seo.OgImageURL,
		&v.CreatedAt, &v.UpdatedAt)
	v.Metrics = nonNil(v.Metrics)
	v.Scope = nonNil(v.Scope)
	v.InitialConditions = nonNil(v.InitialConditions)
	v.Risks = nonNil(v.Risks)
	v.Approach = nonNil(v.Approach)
	v.Solutions = nonNil(v.Solutions)
	v.Impacts = nonNil(v.Impacts)
	return v, err
}

func collectCaseStudies(rows pgx.Rows) ([]domain.CaseStudy, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.CaseStudy, error) { return scanCaseStudy(r) })
}

// ListPublicCaseStudies returns published case studies by sort_order.
func (s *Store) ListPublicCaseStudies(ctx context.Context, featured *bool, category string) ([]domain.CaseStudy, error) {
	var w where
	w.add("status = 'published'")
	if featured != nil {
		w.add("featured = $?", *featured)
	}
	if category != "" {
		w.add("lower(category) = lower($?)", category)
	}
	rows, err := s.db.Query(ctx, "SELECT "+caseStudyCols+" FROM case_studies"+w.sql()+" ORDER BY sort_order, created_at", w.args...)
	if err != nil {
		return nil, fmt.Errorf("store: list case studies: %w", err)
	}
	list, err := collectCaseStudies(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list case studies: %w", err)
	}
	return list, nil
}

// AdminCaseStudyFilter filters the CMS table.
type AdminCaseStudyFilter struct {
	Q      string
	Status string
	Page   Page
}

// ListAdminCaseStudies returns a page ordered by updated_at desc.
func (s *Store) ListAdminCaseStudies(ctx context.Context, f AdminCaseStudyFilter) ([]domain.CaseStudy, int, error) {
	var w where
	if f.Q != "" {
		w.add("(title ILIKE $? OR system_name ILIKE $? OR category ILIKE $?)", likePattern(f.Q))
	}
	if f.Status != "" {
		w.add("status = $?", f.Status)
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM case_studies"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count case studies: %w", err)
	}
	limit := w.next(f.Page.Size)
	offset := w.next(f.Page.offset())
	rows, err := s.db.Query(ctx, "SELECT "+caseStudyCols+" FROM case_studies"+w.sql()+
		" ORDER BY updated_at DESC, id LIMIT "+limit+" OFFSET "+offset, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list case studies: %w", err)
	}
	list, err := collectCaseStudies(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list case studies: %w", err)
	}
	return list, total, nil
}

// GetCaseStudy returns a case study by id.
func (s *Store) GetCaseStudy(ctx context.Context, id string) (*domain.CaseStudy, error) {
	v, err := scanCaseStudy(s.db.QueryRow(ctx, "SELECT "+caseStudyCols+" FROM case_studies WHERE id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get case study: %w", err)
	}
	return &v, nil
}

// GetPublishedCaseStudyBySlug returns a published case study.
func (s *Store) GetPublishedCaseStudyBySlug(ctx context.Context, slug string) (*domain.CaseStudy, error) {
	v, err := scanCaseStudy(s.db.QueryRow(ctx, "SELECT "+caseStudyCols+" FROM case_studies WHERE slug = $1 AND status = 'published'", slug))
	if err != nil {
		return nil, fmt.Errorf("store: get case study: %w", err)
	}
	return &v, nil
}

// MaxFeatured is the maximum number of featured published case studies.
const MaxFeatured = 2

// checkFeatured enforces "at most 2 featured published case studies"
// under an advisory lock (only when the saved row would count).
func (s *Store) checkFeatured(ctx context.Context, id string, v *domain.CaseStudy) error {
	if !v.Featured || v.Status != domain.StatusPublished {
		return nil
	}
	if _, err := s.db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('case_studies_featured'))"); err != nil {
		return fmt.Errorf("store: featured lock: %w", err)
	}
	var n int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM case_studies WHERE featured AND status = 'published' AND id::text <> $1", id).Scan(&n); err != nil {
		return fmt.Errorf("store: featured count: %w", err)
	}
	if n >= MaxFeatured {
		msg := "Maksimal 2 studi kasus unggulan yang terbit. Nonaktifkan unggulan pada studi kasus lain terlebih dahulu."
		return &domain.ConflictError{Code: "conflict", Message: msg, Fields: map[string]string{"featured": msg}}
	}
	return nil
}

// SaveCaseStudy creates (id == "") or fully updates a case study.
// sortOrder: nil → max+1 on create, unchanged on update.
func (s *Store) SaveCaseStudy(ctx context.Context, id string, v *domain.CaseStudy, sortOrder *int) (*domain.CaseStudy, error) {
	var savedID string
	err := s.Tx(ctx, func(tx *Store) error {
		slug, err := tx.resolveSlug(ctx, "case_studies", id, v.Slug, v.Title)
		if err != nil {
			return err
		}
		if err := tx.checkFeatured(ctx, id, v); err != nil {
			return err
		}
		args := []any{slug, v.Status, v.SystemName, v.Category, v.Featured, v.Title, v.Summary, v.CardBody,
			nonNil(v.Metrics), nonNil(v.Scope), v.InitialConditionsTitle, nonNil(v.InitialConditions),
			v.RisksTitle, nonNil(v.Risks), v.ApproachTitle, nonNil(v.Approach), nonNil(v.Solutions),
			v.ImpactTitle, nonNil(v.Impacts), v.Lesson, v.Seo.Title, v.Seo.Description, v.Seo.OgImageURL}
		if id == "" {
			order := 0
			if sortOrder != nil {
				order = *sortOrder
			} else if order, err = tx.nextSortOrder(ctx, "case_studies"); err != nil {
				return err
			}
			args = append(args, order)
			err = tx.db.QueryRow(ctx, `INSERT INTO case_studies (slug, status, system_name, category,
				featured, title, summary, card_body, metrics, scope, initial_conditions_title,
				initial_conditions, risks_title, risks, approach_title, approach, solutions, impact_title,
				impacts, lesson, seo_title, seo_description, og_image_url, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24) RETURNING id::text`, args...).Scan(&savedID)
			if err != nil {
				return fmt.Errorf("store: insert case study: %w", err)
			}
			return nil
		}
		args = append(args, sortOrder, id)
		tag, err := tx.db.Exec(ctx, `UPDATE case_studies SET slug = $1, status = $2, system_name = $3,
			category = $4, featured = $5, title = $6, summary = $7, card_body = $8, metrics = $9,
			scope = $10, initial_conditions_title = $11, initial_conditions = $12, risks_title = $13,
			risks = $14, approach_title = $15, approach = $16, solutions = $17, impact_title = $18,
			impacts = $19, lesson = $20, seo_title = $21, seo_description = $22, og_image_url = $23,
			sort_order = COALESCE($24, sort_order), updated_at = now() WHERE id = $25`, args...)
		if err != nil {
			return fmt.Errorf("store: update case study: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("store: update case study: %w", pgx.ErrNoRows)
		}
		savedID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetCaseStudy(ctx, savedID)
}

// DeleteCaseStudy deletes a case study.
func (s *Store) DeleteCaseStudy(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "case_studies", id)
}
