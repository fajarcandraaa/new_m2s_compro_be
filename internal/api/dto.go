package api

import (
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

// Response DTOs. Nullable fields are pointers and always serialized
// (no omitempty); lists are never nil.

type seoDTO struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	OgImageURL  *string `json:"ogImageUrl"`
}

func toSeo(s domain.Seo) seoDTO {
	return seoDTO{Title: s.Title, Description: s.Description, OgImageURL: s.OgImageURL}
}

type siteDTO struct {
	CompanyName      string  `json:"companyName"`
	LegalName        string  `json:"legalName"`
	Tagline          string  `json:"tagline"`
	Email            string  `json:"email"`
	WhatsappNumber   string  `json:"whatsappNumber"`
	WhatsappDisplay  string  `json:"whatsappDisplay"`
	WhatsappGreeting string  `json:"whatsappGreeting"`
	Address          string  `json:"address"`
	MapsURL          *string `json:"mapsUrl"`
	LinkedinURL      *string `json:"linkedinUrl"`
	InstagramURL     *string `json:"instagramUrl"`
	ShowTestimonials bool    `json:"showTestimonials"`
	ShowClientLogos  bool    `json:"showClientLogos"`
}

type settingsAdminDTO struct {
	siteDTO
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func toSite(s *domain.Settings) siteDTO {
	return siteDTO{
		CompanyName: s.CompanyName, LegalName: s.LegalName, Tagline: s.Tagline, Email: s.Email,
		WhatsappNumber: s.WhatsappNumber, WhatsappDisplay: s.WhatsappDisplay,
		WhatsappGreeting: s.WhatsappGreeting, Address: s.Address, MapsURL: s.MapsURL,
		LinkedinURL: s.LinkedinURL, InstagramURL: s.InstagramURL,
		ShowTestimonials: s.ShowTestimonials, ShowClientLogos: s.ShowClientLogos,
	}
}

func toSettingsAdmin(s *domain.Settings) settingsAdminDTO {
	return settingsAdminDTO{siteDTO: toSite(s), UpdatedAt: httpx.Time(s.UpdatedAt)}
}

type deliverableDTO struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type packageDTO struct {
	Name        string   `json:"name"`
	PriceLabel  string   `json:"priceLabel"`
	PriceValue  string   `json:"priceValue"`
	Description string   `json:"description"`
	Features    []string `json:"features"`
	Highlighted bool     `json:"highlighted"`
	SortOrder   int      `json:"sortOrder"`
}

type serviceDTO struct {
	Slug         string           `json:"slug"`
	Audience     string           `json:"audience"`
	SortOrder    int              `json:"sortOrder"`
	Eyebrow      string           `json:"eyebrow"`
	Title        string           `json:"title"`
	Tagline      string           `json:"tagline"`
	Summary      string           `json:"summary"`
	Points       []string         `json:"points"`
	PriceLabel   string           `json:"priceLabel"`
	PriceValue   string           `json:"priceValue"`
	AudienceText string           `json:"audienceText"`
	Deliverables []deliverableDTO `json:"deliverables"`
	Systems      []string         `json:"systems"`
	Packages     []packageDTO     `json:"packages"`
	Seo          seoDTO           `json:"seo"`
}

type serviceAdminDTO struct {
	ID string `json:"id"`
	serviceDTO
	Status    string     `json:"status"`
	CreatedAt httpx.Time `json:"createdAt"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func strs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toService(v *domain.Service) serviceDTO {
	d := serviceDTO{
		Slug: v.Slug, Audience: v.Audience, SortOrder: v.SortOrder, Eyebrow: v.Eyebrow, Title: v.Title,
		Tagline: v.Tagline, Summary: v.Summary, Points: strs(v.Points), PriceLabel: v.PriceLabel,
		PriceValue: v.PriceValue, AudienceText: v.AudienceText, Systems: strs(v.Systems),
		Deliverables: make([]deliverableDTO, len(v.Deliverables)), Packages: make([]packageDTO, len(v.Packages)),
		Seo: toSeo(v.Seo),
	}
	for i, x := range v.Deliverables {
		d.Deliverables[i] = deliverableDTO{Title: x.Title, Body: x.Body}
	}
	for i, p := range v.Packages {
		d.Packages[i] = packageDTO{Name: p.Name, PriceLabel: p.PriceLabel, PriceValue: p.PriceValue,
			Description: p.Description, Features: strs(p.Features), Highlighted: p.Highlighted, SortOrder: p.SortOrder}
	}
	return d
}

func toServiceAdmin(v *domain.Service) serviceAdminDTO {
	return serviceAdminDTO{ID: v.ID, serviceDTO: toService(v), Status: v.Status,
		CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type metricDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type caseStudySummaryDTO struct {
	Slug       string      `json:"slug"`
	SystemName string      `json:"systemName"`
	Category   string      `json:"category"`
	Featured   bool        `json:"featured"`
	SortOrder  int         `json:"sortOrder"`
	Title      string      `json:"title"`
	Summary    string      `json:"summary"`
	CardBody   string      `json:"cardBody"`
	Metrics    []metricDTO `json:"metrics"`
	Scope      []string    `json:"scope"`
}

type caseStudyDTO struct {
	caseStudySummaryDTO
	InitialConditionsTitle string   `json:"initialConditionsTitle"`
	InitialConditions      []string `json:"initialConditions"`
	RisksTitle             string   `json:"risksTitle"`
	Risks                  []string `json:"risks"`
	ApproachTitle          string   `json:"approachTitle"`
	Approach               []string `json:"approach"`
	Solutions              []string `json:"solutions"`
	ImpactTitle            string   `json:"impactTitle"`
	Impacts                []string `json:"impacts"`
	Lesson                 string   `json:"lesson"`
	Seo                    seoDTO   `json:"seo"`
}

type caseStudyAdminDTO struct {
	ID string `json:"id"`
	caseStudyDTO
	Status    string     `json:"status"`
	CreatedAt httpx.Time `json:"createdAt"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func toCaseStudySummary(v *domain.CaseStudy) caseStudySummaryDTO {
	d := caseStudySummaryDTO{Slug: v.Slug, SystemName: v.SystemName, Category: v.Category,
		Featured: v.Featured, SortOrder: v.SortOrder, Title: v.Title, Summary: v.Summary,
		CardBody: v.CardBody, Metrics: make([]metricDTO, len(v.Metrics)), Scope: strs(v.Scope)}
	for i, m := range v.Metrics {
		d.Metrics[i] = metricDTO{Value: m.Value, Label: m.Label}
	}
	return d
}

func toCaseStudy(v *domain.CaseStudy) caseStudyDTO {
	return caseStudyDTO{
		caseStudySummaryDTO:    toCaseStudySummary(v),
		InitialConditionsTitle: v.InitialConditionsTitle, InitialConditions: strs(v.InitialConditions),
		RisksTitle: v.RisksTitle, Risks: strs(v.Risks), ApproachTitle: v.ApproachTitle,
		Approach: strs(v.Approach), Solutions: strs(v.Solutions), ImpactTitle: v.ImpactTitle,
		Impacts: strs(v.Impacts), Lesson: v.Lesson, Seo: toSeo(v.Seo),
	}
}

func toCaseStudyAdmin(v *domain.CaseStudy) caseStudyAdminDTO {
	return caseStudyAdminDTO{ID: v.ID, caseStudyDTO: toCaseStudy(v), Status: v.Status,
		CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type tagDTO struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type tagWithCountDTO struct {
	tagDTO
	ArticleCount int `json:"articleCount"`
}

func toTags(tags []domain.Tag) []tagDTO {
	out := make([]tagDTO, len(tags))
	for i, t := range tags {
		out[i] = tagDTO{Slug: t.Slug, Name: t.Name}
	}
	return out
}

type articleSummaryDTO struct {
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Excerpt       string     `json:"excerpt"`
	CoverImageURL *string    `json:"coverImageUrl"`
	Tags          []tagDTO   `json:"tags"`
	PublishedAt   httpx.Time `json:"publishedAt"`
	ReadMinutes   int        `json:"readMinutes"`
	Author        string     `json:"author"`
}

type articleDTO struct {
	articleSummaryDTO
	ContentHTML string `json:"contentHtml"`
	Seo         seoDTO `json:"seo"`
}

func toArticleSummary(v *domain.Article) articleSummaryDTO {
	d := articleSummaryDTO{Slug: v.Slug, Title: v.Title, Excerpt: v.Excerpt, CoverImageURL: v.CoverImageURL,
		Tags: toTags(v.Tags), ReadMinutes: v.ReadMinutes, Author: v.Author}
	if v.PublishedAt != nil {
		d.PublishedAt = httpx.Time(*v.PublishedAt)
	}
	return d
}

func toArticle(v *domain.Article) articleDTO {
	return articleDTO{articleSummaryDTO: toArticleSummary(v), ContentHTML: v.ContentHTML, Seo: toSeo(v.Seo)}
}

type articleAdminSummaryDTO struct {
	ID          string      `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Status      string      `json:"status"`
	IsScheduled bool        `json:"isScheduled"`
	PublishedAt *httpx.Time `json:"publishedAt"`
	Tags        []tagDTO    `json:"tags"`
	UpdatedAt   httpx.Time  `json:"updatedAt"`
}

func toArticleAdminSummary(v *domain.Article) articleAdminSummaryDTO {
	return articleAdminSummaryDTO{ID: v.ID, Slug: v.Slug, Title: v.Title, Status: v.Status,
		IsScheduled: v.IsScheduled, PublishedAt: httpx.TimePtr(v.PublishedAt), Tags: toTags(v.Tags),
		UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type articleAdminDTO struct {
	ID            string      `json:"id"`
	Slug          string      `json:"slug"`
	Title         string      `json:"title"`
	Excerpt       string      `json:"excerpt"`
	CoverImageURL *string     `json:"coverImageUrl"`
	Tags          []tagDTO    `json:"tags"`
	ReadMinutes   int         `json:"readMinutes"`
	Author        string      `json:"author"`
	PublishedAt   *httpx.Time `json:"publishedAt"`
	ContentHTML   string      `json:"contentHtml"`
	Seo           seoDTO      `json:"seo"`
	Status        string      `json:"status"`
	IsScheduled   bool        `json:"isScheduled"`
	CreatedAt     httpx.Time  `json:"createdAt"`
	UpdatedAt     httpx.Time  `json:"updatedAt"`
}

func toArticleAdmin(v *domain.Article) articleAdminDTO {
	return articleAdminDTO{ID: v.ID, Slug: v.Slug, Title: v.Title, Excerpt: v.Excerpt,
		CoverImageURL: v.CoverImageURL, Tags: toTags(v.Tags), ReadMinutes: v.ReadMinutes, Author: v.Author,
		PublishedAt: httpx.TimePtr(v.PublishedAt), ContentHTML: v.ContentHTML, Seo: toSeo(v.Seo),
		Status: v.Status, IsScheduled: v.IsScheduled, CreatedAt: httpx.Time(v.CreatedAt),
		UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type serviceRefDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type promoDTO struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Code        *string        `json:"code"`
	StartsAt    httpx.Time     `json:"startsAt"`
	EndsAt      httpx.Time     `json:"endsAt"`
	Status      string         `json:"status"`
	Service     *serviceRefDTO `json:"service"`
}

type promoAdminDTO struct {
	promoDTO
	ServiceID *string    `json:"serviceId"`
	CreatedAt httpx.Time `json:"createdAt"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func toPromo(v *domain.Promo) promoDTO {
	d := promoDTO{ID: v.ID, Title: v.Title, Description: v.Description, Code: v.Code,
		StartsAt: httpx.Time(v.StartsAt), EndsAt: httpx.Time(v.EndsAt), Status: v.Status}
	if v.Service != nil {
		d.Service = &serviceRefDTO{Slug: v.Service.Slug, Title: v.Service.Title}
	}
	return d
}

func toPromoAdmin(v *domain.Promo) promoAdminDTO {
	return promoAdminDTO{promoDTO: toPromo(v), ServiceID: v.ServiceID,
		CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type testimonialDTO struct {
	ID         string  `json:"id"`
	ClientName string  `json:"clientName"`
	ClientRole string  `json:"clientRole"`
	Company    *string `json:"company"`
	Quote      string  `json:"quote"`
	SortOrder  int     `json:"sortOrder"`
}

type testimonialAdminDTO struct {
	testimonialDTO
	IsActive  bool       `json:"isActive"`
	CreatedAt httpx.Time `json:"createdAt"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func toTestimonial(v *domain.Testimonial) testimonialDTO {
	return testimonialDTO{ID: v.ID, ClientName: v.ClientName, ClientRole: v.ClientRole, Company: v.Company,
		Quote: v.Quote, SortOrder: v.SortOrder}
}

func toTestimonialAdmin(v *domain.Testimonial) testimonialAdminDTO {
	return testimonialAdminDTO{testimonialDTO: toTestimonial(v), IsActive: v.IsActive,
		CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type clientLogoDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ImageURL  string `json:"imageUrl"`
	SortOrder int    `json:"sortOrder"`
}

type clientLogoAdminDTO struct {
	clientLogoDTO
	CreatedAt httpx.Time `json:"createdAt"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

func toClientLogo(v *domain.ClientLogo) clientLogoDTO {
	return clientLogoDTO{ID: v.ID, Name: v.Name, ImageURL: v.ImageURL, SortOrder: v.SortOrder}
}

func toClientLogoAdmin(v *domain.ClientLogo) clientLogoAdminDTO {
	return clientLogoAdminDTO{clientLogoDTO: toClientLogo(v), CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type leadDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Company        *string    `json:"company"`
	Email          string     `json:"email"`
	Whatsapp       string     `json:"whatsapp"`
	ServiceSlug    *string    `json:"serviceSlug"`
	ServiceTitle   *string    `json:"serviceTitle"`
	StartTimeframe *string    `json:"startTimeframe"`
	Message        string     `json:"message"`
	Status         string     `json:"status"`
	Notes          *string    `json:"notes"`
	CreatedAt      httpx.Time `json:"createdAt"`
	UpdatedAt      httpx.Time `json:"updatedAt"`
}

func toLead(v *domain.Lead) leadDTO {
	return leadDTO{ID: v.ID, Name: v.Name, Company: v.Company, Email: v.Email, Whatsapp: v.Whatsapp,
		ServiceSlug: v.ServiceSlug, ServiceTitle: v.ServiceTitle, StartTimeframe: v.StartTimeframe,
		Message: v.Message, Status: v.Status, Notes: v.Notes, CreatedAt: httpx.Time(v.CreatedAt),
		UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type leadCreatedDTO struct {
	ID        string     `json:"id"`
	CreatedAt httpx.Time `json:"createdAt"`
}

type userDTO struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Email       string      `json:"email"`
	Role        string      `json:"role"`
	IsActive    bool        `json:"isActive"`
	LastLoginAt *httpx.Time `json:"lastLoginAt"`
	CreatedAt   httpx.Time  `json:"createdAt"`
	UpdatedAt   httpx.Time  `json:"updatedAt"`
}

func toUser(v *domain.User) userDTO {
	return userDTO{ID: v.ID, Name: v.Name, Email: v.Email, Role: v.Role, IsActive: v.IsActive,
		LastLoginAt: httpx.TimePtr(v.LastLoginAt), CreatedAt: httpx.Time(v.CreatedAt), UpdatedAt: httpx.Time(v.UpdatedAt)}
}

type dashboardDTO struct {
	Counts struct {
		NewLeads     int `json:"newLeads"`
		DraftContent int `json:"draftContent"`
		ActivePromos int `json:"activePromos"`
	} `json:"counts"`
	LatestLeads  []leadDTO      `json:"latestLeads"`
	RecentDrafts []draftItemDTO `json:"recentDrafts"`
}

type draftItemDTO struct {
	Type      string     `json:"type"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	UpdatedAt httpx.Time `json:"updatedAt"`
}

type mediaUploadDTO struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// mapSlice converts a slice with fn and never returns nil.
func mapSlice[T, U any](in []T, fn func(*T) U) []U {
	out := make([]U, len(in))
	for i := range in {
		out[i] = fn(&in[i])
	}
	return out
}
