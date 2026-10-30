// Package domain holds entity structs, enums and pure business logic
// (slugify, promo status, read minutes, phone normalisation, validation).
package domain

import "time"

// Roles.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
)

// Content status.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Lead status.
const (
	LeadNew       = "new"
	LeadContacted = "contacted"
	LeadDone      = "done"
)

// DefaultAuthor is used when an article has no author.
const DefaultAuthor = "Tim Mind2Screen"

// Seo holds nullable SEO overrides.
type Seo struct {
	Title       *string
	Description *string
	OgImageURL  *string
}

// Settings is the singleton company-info row.
type Settings struct {
	CompanyName      string
	LegalName        string
	Tagline          string
	Email            string
	WhatsappNumber   string
	WhatsappDisplay  string
	WhatsappGreeting string
	Address          string
	MapsURL          *string
	LinkedinURL      *string
	InstagramURL     *string
	ShowTestimonials bool
	ShowClientLogos  bool
	UpdatedAt        time.Time
}

// Deliverable is one element of services.deliverables (jsonb).
type Deliverable struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Faq is one element of services.faqs / articles.faqs (jsonb). Both
// fields are plain text.
type Faq struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// ServicePackage is a row of service_packages.
type ServicePackage struct {
	Name        string
	PriceLabel  string
	PriceValue  string
	Description string
	Features    []string
	Highlighted bool
	SortOrder   int
}

// Service is a row of services with its packages.
type Service struct {
	ID           string
	Slug         string
	Audience     string
	Status       string
	SortOrder    int
	Eyebrow      string
	Title        string
	Tagline      string
	Summary      string
	PriceLabel   string
	PriceValue   string
	AudienceText string
	Points       []string
	Deliverables []Deliverable
	Systems      []string
	Packages     []ServicePackage
	Faqs         []Faq
	Seo          Seo
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Metric is one case-study impact number (jsonb).
type Metric struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// CaseStudy is a row of case_studies.
type CaseStudy struct {
	ID                     string
	Slug                   string
	Status                 string
	SystemName             string
	Category               string
	Featured               bool
	SortOrder              int
	Title                  string
	Summary                string
	CardBody               string
	Metrics                []Metric
	Scope                  []string
	InitialConditionsTitle string
	InitialConditions      []string
	RisksTitle             string
	Risks                  []string
	ApproachTitle          string
	Approach               []string
	Solutions              []string
	ImpactTitle            string
	Impacts                []string
	Lesson                 string
	Seo                    Seo
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Tag is an article tag.
type Tag struct {
	Slug string
	Name string
}

// TagWithCount is a tag with the number of publicly visible articles.
type TagWithCount struct {
	Tag
	ArticleCount int
}

// Article is a row of articles with its ordered tags.
type Article struct {
	ID            string
	Slug          string
	Status        string
	Title         string
	Excerpt       string
	ContentHTML   string
	CoverImageURL *string
	Author        string
	ReadMinutes   int
	PublishedAt   *time.Time
	Faqs          []Faq
	Seo           Seo
	Tags          []Tag
	IsScheduled   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ServiceRef is the service embedded in a promo.
type ServiceRef struct {
	Slug  string
	Title string
}

// Promo is a row of promos; Status is computed on read.
type Promo struct {
	ID          string
	Title       string
	Description string
	Code        *string
	StartsAt    time.Time
	EndsAt      time.Time
	Status      string
	ServiceID   *string
	Service     *ServiceRef
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Testimonial is a row of testimonials.
type Testimonial struct {
	ID         string
	ClientName string
	ClientRole string
	Company    *string
	Quote      string
	IsActive   bool
	SortOrder  int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ClientLogo is a row of client_logos.
type ClientLogo struct {
	ID        string
	Name      string
	ImageURL  string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Lead is a row of leads joined with the service title.
type Lead struct {
	ID             string
	Name           string
	Company        *string
	Email          string
	Whatsapp       string
	ServiceSlug    *string
	ServiceTitle   *string
	StartTimeframe *string
	Message        string
	Status         string
	Notes          *string
	IPHash         *string
	UserAgent      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// User is a CMS user.
type User struct {
	ID             string
	Name           string
	Email          string
	PasswordHash   string
	Role           string
	IsActive       bool
	SessionVersion int
	LastLoginAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DraftItem is a row of the dashboard "recent drafts" table.
type DraftItem struct {
	Type      string
	ID        string
	Title     string
	UpdatedAt time.Time
}

// Dashboard is the CMS home summary.
type Dashboard struct {
	NewLeads     int
	DraftContent int
	ActivePromos int
	LatestLeads  []Lead
	RecentDrafts []DraftItem
}
