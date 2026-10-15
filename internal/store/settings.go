package store

import (
	"context"
	"fmt"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const settingsCols = `company_name, legal_name, tagline, email, whatsapp_number, whatsapp_display,
	whatsapp_greeting, address, maps_url, linkedin_url, instagram_url, show_testimonials,
	show_client_logos, updated_at`

func scanSettings(row interface{ Scan(...any) error }) (*domain.Settings, error) {
	var st domain.Settings
	err := row.Scan(&st.CompanyName, &st.LegalName, &st.Tagline, &st.Email, &st.WhatsappNumber,
		&st.WhatsappDisplay, &st.WhatsappGreeting, &st.Address, &st.MapsURL, &st.LinkedinURL,
		&st.InstagramURL, &st.ShowTestimonials, &st.ShowClientLogos, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetSettings returns the singleton settings row.
func (s *Store) GetSettings(ctx context.Context) (*domain.Settings, error) {
	st, err := scanSettings(s.db.QueryRow(ctx, "SELECT "+settingsCols+" FROM settings WHERE id = 1"))
	if err != nil {
		return nil, fmt.Errorf("store: get settings: %w", err)
	}
	return st, nil
}

// UpdateSettings replaces company info (not the toggles).
func (s *Store) UpdateSettings(ctx context.Context, in *domain.Settings) (*domain.Settings, error) {
	st, err := scanSettings(s.db.QueryRow(ctx, `UPDATE settings SET company_name = $1, legal_name = $2,
		tagline = $3, email = $4, whatsapp_number = $5, whatsapp_display = $6, whatsapp_greeting = $7,
		address = $8, maps_url = $9, linkedin_url = $10, instagram_url = $11, updated_at = now()
		WHERE id = 1 RETURNING `+settingsCols,
		in.CompanyName, in.LegalName, in.Tagline, in.Email, in.WhatsappNumber, in.WhatsappDisplay,
		in.WhatsappGreeting, in.Address, in.MapsURL, in.LinkedinURL, in.InstagramURL))
	if err != nil {
		return nil, fmt.Errorf("store: update settings: %w", err)
	}
	return st, nil
}

// UpdateToggles updates the homepage toggles; nil = unchanged.
func (s *Store) UpdateToggles(ctx context.Context, showTestimonials, showClientLogos *bool) (*domain.Settings, error) {
	st, err := scanSettings(s.db.QueryRow(ctx, `UPDATE settings SET
		show_testimonials = COALESCE($1, show_testimonials),
		show_client_logos = COALESCE($2, show_client_logos),
		updated_at = now() WHERE id = 1 RETURNING `+settingsCols, showTestimonials, showClientLogos))
	if err != nil {
		return nil, fmt.Errorf("store: update toggles: %w", err)
	}
	return st, nil
}

// InsertSettings creates the singleton row (seed).
func (s *Store) InsertSettings(ctx context.Context, in *domain.Settings) error {
	_, err := s.db.Exec(ctx, `INSERT INTO settings (id, company_name, legal_name, tagline, email,
		whatsapp_number, whatsapp_display, whatsapp_greeting, address, maps_url, linkedin_url,
		instagram_url, show_testimonials, show_client_logos)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		in.CompanyName, in.LegalName, in.Tagline, in.Email, in.WhatsappNumber, in.WhatsappDisplay,
		in.WhatsappGreeting, in.Address, in.MapsURL, in.LinkedinURL, in.InstagramURL,
		in.ShowTestimonials, in.ShowClientLogos)
	if err != nil {
		return fmt.Errorf("store: insert settings: %w", err)
	}
	return nil
}

// Count returns the number of rows in a whitelisted table (seed).
func (s *Store) Count(ctx context.Context, table string) (int, error) {
	allowed := map[string]bool{"users": true, "settings": true, "services": true, "case_studies": true,
		"tags": true, "articles": true, "promos": true, "testimonials": true, "client_logos": true, "leads": true}
	if !allowed[table] {
		return 0, fmt.Errorf("store: count: table %q not allowed", table)
	}
	var n int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count %s: %w", table, err)
	}
	return n, nil
}
