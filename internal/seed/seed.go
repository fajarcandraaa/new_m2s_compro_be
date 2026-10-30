// Package seed loads the initial content from the embedded JSON files. It
// is idempotent per table and runs in one transaction.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/api"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// Admin is the initial admin from ADMIN_* env.
type Admin struct {
	Email    string
	Password string
	Name     string
}

type seedFile struct {
	Settings struct {
		CompanyName      string `json:"company_name"`
		LegalName        string `json:"legal_name"`
		Tagline          string `json:"tagline"`
		Email            string `json:"email"`
		WhatsappNumber   string `json:"whatsapp_number"`
		WhatsappDisplay  string `json:"whatsapp_display"`
		WhatsappGreeting string `json:"whatsapp_greeting"`
		Address          string `json:"address"`
		MapsURL          string `json:"maps_url"`
		LinkedinURL      string `json:"linkedin_url"`
		InstagramURL     string `json:"instagram_url"`
		ShowTestimonials bool   `json:"show_testimonials"`
		ShowClientLogos  bool   `json:"show_client_logos"`
	} `json:"settings"`
	Services []struct {
		Slug         string   `json:"slug"`
		Audience     string   `json:"audience"`
		SortOrder    int      `json:"sort_order"`
		Eyebrow      string   `json:"eyebrow"`
		Title        string   `json:"title"`
		Tagline      string   `json:"tagline"`
		Summary      string   `json:"summary"`
		Points       []string `json:"points"`
		PriceLabel   string   `json:"price_label"`
		PriceValue   string   `json:"price_value"`
		AudienceText string   `json:"audience_text"`
		Deliverables []struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"deliverables"`
		Systems  []string `json:"systems"`
		Packages []struct {
			Name        string   `json:"name"`
			PriceLabel  string   `json:"price_label"`
			PriceValue  string   `json:"price_value"`
			Description string   `json:"description"`
			Features    []string `json:"features"`
			Highlighted bool     `json:"highlighted"`
			SortOrder   int      `json:"sort_order"`
		} `json:"packages"`
		Faqs []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"faqs"`
	} `json:"services"`
	CaseStudies []struct {
		Slug       string `json:"slug"`
		SystemName string `json:"system_name"`
		Category   string `json:"category"`
		Featured   bool   `json:"featured"`
		SortOrder  int    `json:"sort_order"`
		Title      string `json:"title"`
		Summary    string `json:"summary"`
		CardBody   string `json:"card_body"`
		Metrics    []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"metrics"`
		Scope                  []string `json:"scope"`
		InitialConditionsTitle string   `json:"initial_conditions_title"`
		InitialConditions      []string `json:"initial_conditions"`
		RisksTitle             string   `json:"risks_title"`
		Risks                  []string `json:"risks"`
		ApproachTitle          string   `json:"approach_title"`
		Approach               []string `json:"approach"`
		Solutions              []string `json:"solutions"`
		ImpactTitle            string   `json:"impact_title"`
		Impacts                []string `json:"impacts"`
		Lesson                 string   `json:"lesson"`
	} `json:"case_studies"`
	Tags []string `json:"tags"`
}

type seedArticle struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Excerpt     string   `json:"excerpt"`
	Tags        []string `json:"tags"`
	PublishedAt string   `json:"published_at"`
	ReadMinutes int      `json:"read_minutes"`
	Author      string   `json:"author"`
	ContentHTML string   `json:"content_html"`
}

func strPtr(s string) *string { return &s }

// Run seeds every empty table from content (docs/content/seed.json and
// articles.json) in one transaction, and backfills service FAQs that are
// still empty.
func Run(ctx context.Context, st *store.Store, content fs.FS, admin Admin) error {
	var sf seedFile
	if err := readJSON(content, "docs/content/seed.json", &sf); err != nil {
		return err
	}
	var articles []seedArticle
	if err := readJSON(content, "docs/content/articles.json", &articles); err != nil {
		return err
	}
	return st.Tx(ctx, func(tx *store.Store) error {
		if err := seedAdmin(ctx, tx, admin); err != nil {
			return err
		}
		if err := seedSettings(ctx, tx, &sf); err != nil {
			return err
		}
		if err := seedServices(ctx, tx, &sf); err != nil {
			return err
		}
		if err := seedCaseStudies(ctx, tx, &sf); err != nil {
			return err
		}
		return seedArticles(ctx, tx, sf.Tags, articles)
	})
}

func readJSON(content fs.FS, name string, dst any) error {
	b, err := fs.ReadFile(content, name)
	if err != nil {
		return fmt.Errorf("seed: read %s: %w", name, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("seed: parse %s: %w", name, err)
	}
	return nil
}

func empty(ctx context.Context, tx *store.Store, table string) (bool, error) {
	n, err := tx.Count(ctx, table)
	return n == 0, err
}

func seedAdmin(ctx context.Context, tx *store.Store, admin Admin) error {
	ok, err := empty(ctx, tx, "users")
	if err != nil || !ok {
		return err
	}
	if admin.Email == "" || admin.Password == "" {
		slog.Warn("seed: ADMIN_EMAIL / ADMIN_PASSWORD not set; no admin user created")
		return nil
	}
	if !domain.IsEmail(admin.Email) {
		slog.Warn("seed: ADMIN_EMAIL is not a valid email; no admin user created")
		return nil
	}
	if msg := auth.PasswordProblem(admin.Password); msg != "" {
		slog.Warn("seed: ADMIN_PASSWORD must be 10-64 characters and at most 72 bytes; no admin user created")
		return nil
	}
	name := domain.Trim(admin.Name)
	if name == "" {
		name = "Admin"
	}
	hash, err := auth.HashPassword(admin.Password)
	if err != nil {
		return err
	}
	if _, err := tx.CreateUser(ctx, name, admin.Email, domain.RoleAdmin, hash); err != nil {
		return err
	}
	slog.Info("seed: admin user created")
	return nil
}

func seedSettings(ctx context.Context, tx *store.Store, sf *seedFile) error {
	ok, err := empty(ctx, tx, "settings")
	if err != nil || !ok {
		return err
	}
	in := api.SettingsInput{
		CompanyName: sf.Settings.CompanyName, LegalName: sf.Settings.LegalName, Tagline: sf.Settings.Tagline,
		Email: sf.Settings.Email, WhatsappNumber: sf.Settings.WhatsappNumber,
		WhatsappDisplay: sf.Settings.WhatsappDisplay, WhatsappGreeting: sf.Settings.WhatsappGreeting,
		Address: sf.Settings.Address, MapsURL: strPtr(sf.Settings.MapsURL),
		LinkedinURL: strPtr(sf.Settings.LinkedinURL), InstagramURL: strPtr(sf.Settings.InstagramURL),
	}
	v, err := in.Validate()
	if err != nil {
		return fmt.Errorf("seed: settings: %w", err)
	}
	v.ShowTestimonials = sf.Settings.ShowTestimonials
	v.ShowClientLogos = sf.Settings.ShowClientLogos
	if err := tx.InsertSettings(ctx, v); err != nil {
		return err
	}
	slog.Info("seed: settings created")
	return nil
}

// seedServices creates the services when the table is empty. On an
// existing database it only backfills FAQs (contract v1.1) into services
// whose faqs are still [], so admin edits are never overwritten.
func seedServices(ctx context.Context, tx *store.Store, sf *seedFile) error {
	fresh, err := empty(ctx, tx, "services")
	if err != nil {
		return err
	}
	backfilled := 0
	for _, s := range sf.Services {
		in := api.ServiceInput{
			Slug: s.Slug, Audience: s.Audience, Eyebrow: s.Eyebrow, Title: s.Title, Tagline: s.Tagline,
			Summary: s.Summary, Points: s.Points, PriceLabel: s.PriceLabel, PriceValue: s.PriceValue,
			AudienceText: s.AudienceText, Systems: s.Systems, Status: domain.StatusPublished,
		}
		for _, d := range s.Deliverables {
			in.Deliverables = append(in.Deliverables, api.DeliverableInput{Title: d.Title, Body: d.Body})
		}
		for _, p := range s.Packages {
			in.Packages = append(in.Packages, api.PackageInput{Name: p.Name, PriceLabel: p.PriceLabel,
				PriceValue: p.PriceValue, Description: p.Description, Features: p.Features, Highlighted: p.Highlighted})
		}
		for _, f := range s.Faqs {
			in.Faqs = append(in.Faqs, api.FaqInput{Question: f.Question, Answer: f.Answer})
		}
		v, err := in.Validate(true)
		if err != nil {
			return fmt.Errorf("seed: service %s: %w", s.Slug, err)
		}
		if !fresh {
			if len(v.Faqs) == 0 {
				continue
			}
			done, err := tx.SetServiceFaqsIfEmpty(ctx, v.Slug, v.Faqs)
			if err != nil {
				return fmt.Errorf("seed: service %s faqs: %w", s.Slug, err)
			}
			if done {
				backfilled++
			}
			continue
		}
		v.SortOrder = s.SortOrder
		if _, err := tx.SaveService(ctx, "", v); err != nil {
			return fmt.Errorf("seed: service %s: %w", s.Slug, err)
		}
	}
	if !fresh {
		if backfilled > 0 {
			slog.Info("seed: service faqs backfilled", "count", backfilled)
		}
		return nil
	}
	slog.Info("seed: services created", "count", len(sf.Services))
	return nil
}

func seedCaseStudies(ctx context.Context, tx *store.Store, sf *seedFile) error {
	ok, err := empty(ctx, tx, "case_studies")
	if err != nil || !ok {
		return err
	}
	for _, c := range sf.CaseStudies {
		in := api.CaseStudyInput{
			Slug: c.Slug, SystemName: c.SystemName, Category: c.Category, Featured: c.Featured,
			Title: c.Title, Summary: c.Summary, CardBody: c.CardBody, Scope: c.Scope,
			InitialConditionsTitle: c.InitialConditionsTitle, InitialConditions: c.InitialConditions,
			RisksTitle: c.RisksTitle, Risks: c.Risks, ApproachTitle: c.ApproachTitle, Approach: c.Approach,
			Solutions: c.Solutions, ImpactTitle: c.ImpactTitle, Impacts: c.Impacts, Lesson: c.Lesson,
			Status: domain.StatusPublished,
		}
		for _, m := range c.Metrics {
			in.Metrics = append(in.Metrics, api.MetricInput{Value: m.Value, Label: m.Label})
		}
		v, err := in.Validate(true)
		if err != nil {
			return fmt.Errorf("seed: case study %s: %w", c.Slug, err)
		}
		order := c.SortOrder
		if _, err := tx.SaveCaseStudy(ctx, "", v, &order); err != nil {
			return fmt.Errorf("seed: case study %s: %w", c.Slug, err)
		}
	}
	slog.Info("seed: case studies created", "count", len(sf.CaseStudies))
	return nil
}

func seedArticles(ctx context.Context, tx *store.Store, tagNames []string, articles []seedArticle) error {
	ok, err := empty(ctx, tx, "tags")
	if err != nil {
		return err
	}
	if ok {
		tags := make([]domain.Tag, 0, len(tagNames))
		for _, n := range tagNames {
			n = domain.Trim(n)
			if slug := domain.Slugify(n); slug != "" {
				tags = append(tags, domain.Tag{Slug: slug, Name: n})
			}
		}
		if err := tx.InsertTags(ctx, tags); err != nil {
			return err
		}
	}
	ok, err = empty(ctx, tx, "articles")
	if err != nil || !ok {
		return err
	}
	for _, a := range articles {
		pub := a.PublishedAt
		in := api.ArticleInput{Title: a.Title, Slug: a.Slug, Excerpt: a.Excerpt, ContentHTML: a.ContentHTML,
			Status: domain.StatusPublished, PublishedAt: &pub, Tags: a.Tags, Author: a.Author}
		v, err := in.Validate(true, time.Now())
		if err != nil {
			return fmt.Errorf("seed: article %s: %w", a.Slug, err)
		}
		if a.ReadMinutes >= 1 {
			v.ReadMinutes = a.ReadMinutes // seed values are kept as given
		}
		if _, err := tx.SaveArticle(ctx, "", v); err != nil {
			return fmt.Errorf("seed: article %s: %w", a.Slug, err)
		}
	}
	slog.Info("seed: articles created", "count", len(articles))
	return nil
}
