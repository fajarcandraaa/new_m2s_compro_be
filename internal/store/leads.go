package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const leadCols = `l.id, l.name, l.company, l.email, l.whatsapp, l.service_slug, s.title, l.start_timeframe,
	l.message, l.status, l.notes, l.created_at, l.updated_at`

const leadFrom = ` FROM leads l LEFT JOIN services s ON s.slug = l.service_slug`

func scanLead(row pgx.Row) (domain.Lead, error) {
	var v domain.Lead
	err := row.Scan(&v.ID, &v.Name, &v.Company, &v.Email, &v.Whatsapp, &v.ServiceSlug, &v.ServiceTitle,
		&v.StartTimeframe, &v.Message, &v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func collectLeads(rows pgx.Rows) ([]domain.Lead, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Lead, error) { return scanLead(r) })
}

// LeadFilter filters leads (list and export).
type LeadFilter struct {
	Q      string
	Status string
	Page   *Page // nil = unpaginated
}

func (f LeadFilter) where() *where {
	w := &where{}
	if f.Q != "" {
		w.add(`(l.name ILIKE $? OR l.company ILIKE $? OR l.email ILIKE $? OR l.whatsapp ILIKE $?
			OR l.message ILIKE $?)`, likePattern(f.Q))
	}
	if f.Status != "" {
		w.add("l.status = $?", f.Status)
	}
	return w
}

// ListLeads returns leads by created_at desc and the total count.
func (s *Store) ListLeads(ctx context.Context, f LeadFilter) ([]domain.Lead, int, error) {
	w := f.where()
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM leads l"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count leads: %w", err)
	}
	q := "SELECT " + leadCols + leadFrom + w.sql() + " ORDER BY l.created_at DESC, l.id"
	if f.Page != nil {
		q += " LIMIT " + w.next(f.Page.Size) + " OFFSET " + w.next(f.Page.offset())
	}
	rows, err := s.db.Query(ctx, q, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list leads: %w", err)
	}
	list, err := collectLeads(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list leads: %w", err)
	}
	return list, total, nil
}

// GetLead returns a lead by id.
func (s *Store) GetLead(ctx context.Context, id string) (*domain.Lead, error) {
	v, err := scanLead(s.db.QueryRow(ctx, "SELECT "+leadCols+leadFrom+" WHERE l.id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("store: get lead: %w", err)
	}
	return &v, nil
}

// CreateLead inserts a lead and returns its id and created_at.
func (s *Store) CreateLead(ctx context.Context, v *domain.Lead) error {
	err := s.db.QueryRow(ctx, `INSERT INTO leads (name, company, email, whatsapp, service_slug,
		start_timeframe, message, ip_hash, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id::text, created_at`,
		v.Name, v.Company, v.Email, v.Whatsapp, v.ServiceSlug, v.StartTimeframe, v.Message, v.IPHash,
		v.UserAgent).Scan(&v.ID, &v.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: insert lead: %w", err)
	}
	return nil
}

// PatchLead updates status and/or notes. setNotes=false keeps notes.
func (s *Store) PatchLead(ctx context.Context, id string, status *string, setNotes bool, notes *string) (*domain.Lead, error) {
	tag, err := s.db.Exec(ctx, `UPDATE leads SET status = COALESCE($2, status),
		notes = CASE WHEN $3 THEN $4 ELSE notes END, updated_at = now() WHERE id = $1`,
		id, status, setNotes, notes)
	if err != nil {
		return nil, fmt.Errorf("store: patch lead: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("store: patch lead: %w", pgx.ErrNoRows)
	}
	return s.GetLead(ctx, id)
}

// Dashboard returns the CMS home summary.
func (s *Store) Dashboard(ctx context.Context) (*domain.Dashboard, error) {
	d := &domain.Dashboard{}
	err := s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM leads WHERE status = 'new'),
		(SELECT count(*) FROM articles WHERE status = 'draft')
		  + (SELECT count(*) FROM services WHERE status = 'draft')
		  + (SELECT count(*) FROM case_studies WHERE status = 'draft'),
		(SELECT count(*) FROM promos WHERE starts_at <= now() AND now() < ends_at)`).
		Scan(&d.NewLeads, &d.DraftContent, &d.ActivePromos)
	if err != nil {
		return nil, fmt.Errorf("store: dashboard counts: %w", err)
	}
	rows, err := s.db.Query(ctx, "SELECT "+leadCols+leadFrom+" ORDER BY l.created_at DESC, l.id LIMIT 5")
	if err != nil {
		return nil, fmt.Errorf("store: dashboard leads: %w", err)
	}
	if d.LatestLeads, err = collectLeads(rows); err != nil {
		return nil, fmt.Errorf("store: dashboard leads: %w", err)
	}
	rows, err = s.db.Query(ctx, `SELECT type, id, title, updated_at FROM (
		SELECT 'article' AS type, id::text, title, updated_at FROM articles WHERE status = 'draft'
		UNION ALL SELECT 'service', id::text, title, updated_at FROM services WHERE status = 'draft'
		UNION ALL SELECT 'case_study', id::text, title, updated_at FROM case_studies WHERE status = 'draft'
	) d ORDER BY updated_at DESC, id LIMIT 5`)
	if err != nil {
		return nil, fmt.Errorf("store: dashboard drafts: %w", err)
	}
	d.RecentDrafts, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.DraftItem, error) {
		var it domain.DraftItem
		err := r.Scan(&it.Type, &it.ID, &it.Title, &it.UpdatedAt)
		return it, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: dashboard drafts: %w", err)
	}
	return d, nil
}
