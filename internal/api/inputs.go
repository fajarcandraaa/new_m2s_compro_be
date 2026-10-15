package api

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
)

// Request DTOs. POST/PUT use full-object semantics (a missing key is its
// zero value); every Validate trims strings and maps "" to nil for
// nullable fields before checking the limits from openapi.yaml.

type SeoInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	OgImageURL  *string `json:"ogImageUrl"`
}

func (s SeoInput) domain() domain.Seo {
	return domain.TrimSeo(domain.Seo{Title: s.Title, Description: s.Description, OgImageURL: s.OgImageURL})
}

func checkSlug(v *domain.Validator, slug string) {
	if slug != "" && !domain.IsSlug(slug) {
		if len(slug) > domain.SlugMaxLen {
			v.Add("slug", domain.MaxLenMsg(domain.SlugMaxLen))
			return
		}
		v.Add("slug", domain.MsgSlug)
	}
}

func checkTitleSlug(v *domain.Validator, slug, title string, creating bool) {
	if creating && slug == "" && title != "" && domain.Slugify(title) == "" {
		v.Add("title", domain.MsgTitleNoSlug)
	}
}

// FaqInput is one element of ServiceInput.faqs / ArticleInput.faqs.
type FaqInput struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// checkFaqs validates FAQ items (max 10, order kept). Both fields are plain
// text: markup is stripped before trimming, so a value that is only HTML
// becomes "" and fails as required. A missing or null list is [].
func checkFaqs(v *domain.Validator, in []FaqInput) []domain.Faq {
	out := make([]domain.Faq, len(in))
	if !v.MaxItems("faqs", len(in), 10) {
		return out
	}
	for i, f := range in {
		out[i] = domain.Faq{Question: domain.Trim(domain.StripHTML(f.Question)), Answer: domain.Trim(domain.StripHTML(f.Answer))}
		v.Str(fmt.Sprintf("faqs.%d.question", i), out[i].Question, true, 200)
		v.Str(fmt.Sprintf("faqs.%d.answer", i), out[i].Answer, true, 1000)
	}
	return out
}

// ---- settings ----

type SettingsInput struct {
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
}

// Validate normalises and validates the input into a domain value.
func (in *SettingsInput) Validate() (*domain.Settings, error) {
	s := &domain.Settings{
		CompanyName: domain.Trim(in.CompanyName), LegalName: domain.Trim(in.LegalName),
		Tagline: domain.Trim(in.Tagline), Email: domain.Trim(in.Email),
		WhatsappDisplay: domain.Trim(in.WhatsappDisplay), WhatsappGreeting: domain.Trim(in.WhatsappGreeting),
		Address: domain.Trim(in.Address), MapsURL: domain.TrimPtr(in.MapsURL),
		LinkedinURL: domain.TrimPtr(in.LinkedinURL), InstagramURL: domain.TrimPtr(in.InstagramURL),
	}
	v := domain.NewValidator()
	v.Str("companyName", s.CompanyName, true, 120)
	v.Str("legalName", s.LegalName, true, 160)
	v.Str("tagline", s.Tagline, false, 160)
	v.Email("email", s.Email)
	s.WhatsappNumber = phone(v, "whatsappNumber", in.WhatsappNumber)
	v.Str("whatsappDisplay", s.WhatsappDisplay, false, 32)
	v.Str("whatsappGreeting", s.WhatsappGreeting, false, 300)
	v.Str("address", s.Address, false, 300)
	v.HTTPURL("mapsUrl", s.MapsURL)
	v.HTTPURL("linkedinUrl", s.LinkedinURL)
	v.HTTPURL("instagramUrl", s.InstagramURL)
	return s, v.Err()
}

var phoneInputRe = regexp.MustCompile(`^\+?[0-9][0-9 ()-]{6,23}[0-9]$`)

// phone validates a phone number as typed and returns it in E.164.
func phone(v *domain.Validator, field, raw string) string {
	raw = domain.Trim(raw)
	if raw == "" {
		v.Add(field, domain.MsgRequired)
		return ""
	}
	if utf8.RuneCountInString(raw) > 25 || !phoneInputRe.MatchString(raw) {
		v.Add(field, domain.MsgPhone)
		return ""
	}
	n, ok := domain.NormalizePhone(raw)
	if !ok {
		v.Add(field, domain.MsgPhone)
		return ""
	}
	return n
}

type TogglesInput struct {
	ShowTestimonials httpx.Optional[bool] `json:"showTestimonials"`
	ShowClientLogos  httpx.Optional[bool] `json:"showClientLogos"`
}

func (in *TogglesInput) Validate() (showT, showL *bool, err error) {
	v := domain.NewValidator()
	if !in.ShowTestimonials.Set && !in.ShowClientLogos.Set {
		return nil, nil, &domain.ValidationError{Message: "Kirim minimal satu pengaturan.", Fields: map[string]string{}}
	}
	if in.ShowTestimonials.Set {
		if in.ShowTestimonials.Null {
			v.Add("showTestimonials", domain.MsgInvalid)
		} else {
			showT = &in.ShowTestimonials.Value
		}
	}
	if in.ShowClientLogos.Set {
		if in.ShowClientLogos.Null {
			v.Add("showClientLogos", domain.MsgInvalid)
		} else {
			showL = &in.ShowClientLogos.Value
		}
	}
	return showT, showL, v.Err()
}

// ---- services ----

type DeliverableInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type PackageInput struct {
	Name        string   `json:"name"`
	PriceLabel  string   `json:"priceLabel"`
	PriceValue  string   `json:"priceValue"`
	Description string   `json:"description"`
	Features    []string `json:"features"`
	Highlighted bool     `json:"highlighted"`
}

type ServiceInput struct {
	Slug         string             `json:"slug"`
	Audience     string             `json:"audience"`
	Eyebrow      string             `json:"eyebrow"`
	Title        string             `json:"title"`
	Tagline      string             `json:"tagline"`
	Summary      string             `json:"summary"`
	Points       []string           `json:"points"`
	PriceLabel   string             `json:"priceLabel"`
	PriceValue   string             `json:"priceValue"`
	AudienceText string             `json:"audienceText"`
	Deliverables []DeliverableInput `json:"deliverables"`
	Systems      []string           `json:"systems"`
	Packages     []PackageInput     `json:"packages"`
	Faqs         []FaqInput         `json:"faqs"`
	Seo          SeoInput           `json:"seo"`
	Status       string             `json:"status"`
}

func (in *ServiceInput) Validate(creating bool) (*domain.Service, error) {
	s := &domain.Service{
		Slug: domain.Trim(in.Slug), Audience: domain.Trim(in.Audience), Status: domain.Trim(in.Status),
		Eyebrow: domain.Trim(in.Eyebrow), Title: domain.Trim(in.Title), Tagline: domain.Trim(in.Tagline),
		Summary: domain.Trim(in.Summary), PriceLabel: domain.Trim(in.PriceLabel),
		PriceValue: domain.Trim(in.PriceValue), AudienceText: domain.Trim(in.AudienceText),
		Points: domain.TrimList(in.Points), Systems: domain.TrimList(in.Systems), Seo: in.Seo.domain(),
		Deliverables: make([]domain.Deliverable, len(in.Deliverables)),
		Packages:     make([]domain.ServicePackage, len(in.Packages)),
	}
	v := domain.NewValidator()
	checkSlug(v, s.Slug)
	v.Enum("audience", s.Audience, "business", "personal")
	v.Str("eyebrow", s.Eyebrow, false, 60)
	v.Str("title", s.Title, true, 120)
	checkTitleSlug(v, s.Slug, s.Title, creating)
	v.Str("tagline", s.Tagline, false, 200)
	v.Str("summary", s.Summary, false, 400)
	v.StrList("points", s.Points, 10, 160)
	v.Str("priceLabel", s.PriceLabel, false, 40)
	v.Str("priceValue", s.PriceValue, false, 60)
	v.Str("audienceText", s.AudienceText, false, 600)
	if v.MaxItems("deliverables", len(in.Deliverables), 12) {
		for i, d := range in.Deliverables {
			s.Deliverables[i] = domain.Deliverable{Title: domain.Trim(d.Title), Body: domain.Trim(d.Body)}
			v.Str(fmt.Sprintf("deliverables.%d.title", i), s.Deliverables[i].Title, true, 160)
			v.Str(fmt.Sprintf("deliverables.%d.body", i), s.Deliverables[i].Body, false, 600)
		}
	}
	v.StrList("systems", s.Systems, 20, 60)
	if v.MaxItems("packages", len(in.Packages), 8) {
		highlighted := 0
		for i, p := range in.Packages {
			pk := domain.ServicePackage{Name: domain.Trim(p.Name), PriceLabel: domain.Trim(p.PriceLabel),
				PriceValue: domain.Trim(p.PriceValue), Description: domain.Trim(p.Description),
				Features: domain.TrimList(p.Features), Highlighted: p.Highlighted, SortOrder: i + 1}
			s.Packages[i] = pk
			prefix := fmt.Sprintf("packages.%d.", i)
			v.Str(prefix+"name", pk.Name, true, 60)
			v.Str(prefix+"priceLabel", pk.PriceLabel, false, 40)
			v.Str(prefix+"priceValue", pk.PriceValue, true, 60)
			v.Str(prefix+"description", pk.Description, false, 300)
			v.StrList(prefix+"features", pk.Features, 12, 120)
			if pk.Highlighted {
				highlighted++
			}
		}
		if highlighted > 1 {
			v.Add("packages", "Hanya satu paket yang boleh ditandai unggulan.")
		}
	}
	s.Faqs = checkFaqs(v, in.Faqs)
	v.Seo(s.Seo)
	v.Enum("status", s.Status, domain.StatusDraft, domain.StatusPublished)
	return s, v.Err()
}

// ---- case studies ----

type MetricInput struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type CaseStudyInput struct {
	Slug                   string        `json:"slug"`
	SystemName             string        `json:"systemName"`
	Category               string        `json:"category"`
	Featured               bool          `json:"featured"`
	SortOrder              *int          `json:"sortOrder"`
	Title                  string        `json:"title"`
	Summary                string        `json:"summary"`
	CardBody               string        `json:"cardBody"`
	Metrics                []MetricInput `json:"metrics"`
	Scope                  []string      `json:"scope"`
	InitialConditionsTitle string        `json:"initialConditionsTitle"`
	InitialConditions      []string      `json:"initialConditions"`
	RisksTitle             string        `json:"risksTitle"`
	Risks                  []string      `json:"risks"`
	ApproachTitle          string        `json:"approachTitle"`
	Approach               []string      `json:"approach"`
	Solutions              []string      `json:"solutions"`
	ImpactTitle            string        `json:"impactTitle"`
	Impacts                []string      `json:"impacts"`
	Lesson                 string        `json:"lesson"`
	Seo                    SeoInput      `json:"seo"`
	Status                 string        `json:"status"`
}

func (in *CaseStudyInput) Validate(creating bool) (*domain.CaseStudy, error) {
	c := &domain.CaseStudy{
		Slug: domain.Trim(in.Slug), Status: domain.Trim(in.Status), SystemName: domain.Trim(in.SystemName),
		Category: domain.Trim(in.Category), Featured: in.Featured, Title: domain.Trim(in.Title),
		Summary: domain.Trim(in.Summary), CardBody: domain.Trim(in.CardBody),
		Metrics: make([]domain.Metric, len(in.Metrics)), Scope: domain.TrimList(in.Scope),
		InitialConditionsTitle: domain.Trim(in.InitialConditionsTitle),
		InitialConditions:      domain.TrimList(in.InitialConditions),
		RisksTitle:             domain.Trim(in.RisksTitle), Risks: domain.TrimList(in.Risks),
		ApproachTitle: domain.Trim(in.ApproachTitle), Approach: domain.TrimList(in.Approach),
		Solutions: domain.TrimList(in.Solutions), ImpactTitle: domain.Trim(in.ImpactTitle),
		Impacts: domain.TrimList(in.Impacts), Lesson: domain.Trim(in.Lesson), Seo: in.Seo.domain(),
	}
	v := domain.NewValidator()
	checkSlug(v, c.Slug)
	v.Str("systemName", c.SystemName, true, 60)
	v.Str("category", c.Category, true, 60)
	if in.SortOrder != nil && *in.SortOrder < 0 {
		v.Add("sortOrder", domain.MsgInvalid)
	}
	v.Str("title", c.Title, true, 200)
	checkTitleSlug(v, c.Slug, c.Title, creating)
	v.Str("summary", c.Summary, false, 400)
	v.Str("cardBody", c.CardBody, false, 400)
	if len(in.Metrics) != 2 {
		v.Add("metrics", "Wajib berisi tepat 2 angka dampak.")
	}
	for i, m := range in.Metrics {
		c.Metrics[i] = domain.Metric{Value: domain.Trim(m.Value), Label: domain.Trim(m.Label)}
		if i < 2 {
			v.Str(fmt.Sprintf("metrics.%d.value", i), c.Metrics[i].Value, true, 20)
			v.Str(fmt.Sprintf("metrics.%d.label", i), c.Metrics[i].Label, true, 160)
		}
	}
	v.StrList("scope", c.Scope, 12, 60)
	v.Str("initialConditionsTitle", c.InitialConditionsTitle, false, 200)
	v.StrList("initialConditions", c.InitialConditions, 20, 300)
	v.Str("risksTitle", c.RisksTitle, false, 200)
	v.StrList("risks", c.Risks, 20, 300)
	v.Str("approachTitle", c.ApproachTitle, false, 200)
	v.StrList("approach", c.Approach, 20, 300)
	v.StrList("solutions", c.Solutions, 20, 300)
	v.Str("impactTitle", c.ImpactTitle, false, 200)
	v.StrList("impacts", c.Impacts, 20, 300)
	v.Str("lesson", c.Lesson, false, 600)
	v.Seo(c.Seo)
	v.Enum("status", c.Status, domain.StatusDraft, domain.StatusPublished)
	return c, v.Err()
}

// ---- articles ----

type ArticleInput struct {
	Title         string     `json:"title"`
	Slug          string     `json:"slug"`
	Excerpt       string     `json:"excerpt"`
	ContentHTML   string     `json:"contentHtml"`
	Status        string     `json:"status"`
	PublishedAt   *string    `json:"publishedAt"`
	Tags          []string   `json:"tags"`
	CoverImageURL *string    `json:"coverImageUrl"`
	Author        string     `json:"author"`
	Faqs          []FaqInput `json:"faqs"`
	Seo           SeoInput   `json:"seo"`
}

// Validate normalises the article: sanitises HTML, computes readMinutes,
// dedupes tags by slug and sets publishedAt = now when publishing without
// a date.
func (in *ArticleInput) Validate(creating bool, now time.Time) (*domain.Article, error) {
	a := &domain.Article{
		Slug: domain.Trim(in.Slug), Status: domain.Trim(in.Status), Title: domain.Trim(in.Title),
		Excerpt: domain.Trim(in.Excerpt), CoverImageURL: domain.TrimPtr(in.CoverImageURL),
		Author: domain.Trim(in.Author), Seo: in.Seo.domain(), Tags: []domain.Tag{},
	}
	if a.Author == "" {
		a.Author = domain.DefaultAuthor
	}
	v := domain.NewValidator()
	v.Str("title", a.Title, true, 200)
	checkSlug(v, a.Slug)
	checkTitleSlug(v, a.Slug, a.Title, creating)
	v.Str("excerpt", a.Excerpt, false, 400)
	if utf8.RuneCountInString(in.ContentHTML) > 200000 {
		v.Add("contentHtml", domain.MaxLenMsg(200000))
	} else {
		a.ContentHTML = strings.TrimSpace(domain.SanitizeHTML(in.ContentHTML))
		if utf8.RuneCountInString(a.ContentHTML) > 200000 {
			v.Add("contentHtml", domain.MaxLenMsg(200000))
		}
	}
	a.ReadMinutes = domain.ReadMinutes(a.ContentHTML)
	v.Enum("status", a.Status, domain.StatusDraft, domain.StatusPublished)
	if p := domain.TrimPtr(in.PublishedAt); p != nil {
		t, err := time.Parse(time.RFC3339, *p)
		if err != nil {
			v.Add("publishedAt", domain.MsgDate)
		} else {
			t = t.UTC()
			a.PublishedAt = &t
		}
	}
	if a.Status == domain.StatusPublished && a.PublishedAt == nil && !v.Has("publishedAt") {
		t := now.UTC()
		a.PublishedAt = &t
	}
	if v.MaxItems("tags", len(in.Tags), 10) {
		seen := map[string]bool{}
		for i, raw := range in.Tags {
			name := domain.Trim(raw)
			field := fmt.Sprintf("tags.%d", i)
			v.Str(field, name, true, 60)
			if v.Has(field) {
				continue
			}
			slug := domain.Slugify(name)
			if slug == "" {
				v.Add(field, "Nama tag harus mengandung huruf atau angka.")
				continue
			}
			if seen[slug] {
				continue
			}
			seen[slug] = true
			a.Tags = append(a.Tags, domain.Tag{Slug: slug, Name: name})
		}
	}
	v.MediaURL("coverImageUrl", a.CoverImageURL, false)
	v.Str("author", a.Author, false, 120)
	a.Faqs = checkFaqs(v, in.Faqs)
	v.Seo(a.Seo)
	return a, v.Err()
}

// ---- promos ----

type PromoInput struct {
	Title       string  `json:"title"`
	ServiceID   *string `json:"serviceId"`
	Description string  `json:"description"`
	StartsAt    string  `json:"startsAt"`
	EndsAt      string  `json:"endsAt"`
	Code        *string `json:"code"`
}

var promoCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)

func parseTime(v *domain.Validator, field, raw string) time.Time {
	raw = domain.Trim(raw)
	if raw == "" {
		v.Add(field, domain.MsgRequired)
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		v.Add(field, domain.MsgDate)
		return time.Time{}
	}
	return t.UTC()
}

func (in *PromoInput) Validate() (*domain.Promo, error) {
	p := &domain.Promo{Title: domain.Trim(in.Title), Description: domain.Trim(in.Description),
		Code: domain.TrimPtr(in.Code), ServiceID: domain.TrimPtr(in.ServiceID)}
	v := domain.NewValidator()
	v.Str("title", p.Title, true, 160)
	if p.ServiceID != nil {
		if !httpx.IsUUID(*p.ServiceID) {
			v.Add("serviceId", domain.MsgInvalid)
		} else {
			lower := strings.ToLower(*p.ServiceID)
			p.ServiceID = &lower
		}
	}
	v.Str("description", p.Description, false, 1000)
	p.StartsAt = parseTime(v, "startsAt", in.StartsAt)
	p.EndsAt = parseTime(v, "endsAt", in.EndsAt)
	if !v.Has("startsAt") && !v.Has("endsAt") && !p.EndsAt.After(p.StartsAt) {
		v.Add("endsAt", "Tanggal berakhir harus setelah tanggal mulai.")
	}
	if p.Code != nil {
		v.Str("code", *p.Code, false, 40)
		if !promoCodeRe.MatchString(*p.Code) {
			v.Add("code", "Kode hanya boleh berisi huruf, angka, _ dan -.")
		}
	}
	return p, v.Err()
}

// ---- testimonials & client logos ----

type TestimonialInput struct {
	ClientName string  `json:"clientName"`
	ClientRole string  `json:"clientRole"`
	Company    *string `json:"company"`
	Quote      string  `json:"quote"`
	IsActive   bool    `json:"isActive"`
}

func (in *TestimonialInput) Validate() (*domain.Testimonial, error) {
	t := &domain.Testimonial{ClientName: domain.Trim(in.ClientName), ClientRole: domain.Trim(in.ClientRole),
		Company: domain.TrimPtr(in.Company), Quote: domain.Trim(in.Quote), IsActive: in.IsActive}
	v := domain.NewValidator()
	v.Str("clientName", t.ClientName, true, 120)
	v.Str("clientRole", t.ClientRole, false, 120)
	v.OptStr("company", t.Company, 160)
	v.Str("quote", t.Quote, true, 600)
	return t, v.Err()
}

type ClientLogoInput struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

func (in *ClientLogoInput) Validate() (*domain.ClientLogo, error) {
	c := &domain.ClientLogo{Name: domain.Trim(in.Name), ImageURL: domain.Trim(in.ImageURL)}
	v := domain.NewValidator()
	v.Str("name", c.Name, true, 120)
	v.MediaURL("imageUrl", &c.ImageURL, true)
	return c, v.Err()
}

type ReorderInput struct {
	IDs []string `json:"ids"`
}

func (in *ReorderInput) Validate() ([]string, error) {
	v := domain.NewValidator()
	if len(in.IDs) == 0 {
		v.Add("ids", domain.MsgRequired)
	} else if len(in.IDs) > 500 {
		v.Add("ids", "Maksimal 500 item.")
	}
	ids := make([]string, len(in.IDs))
	for i, id := range in.IDs {
		if !httpx.IsUUID(id) {
			v.Add(fmt.Sprintf("ids.%d", i), domain.MsgInvalid)
		}
		ids[i] = strings.ToLower(id)
	}
	return ids, v.Err()
}

// ---- leads ----

type LeadCreateInput struct {
	Name           string  `json:"name"`
	Company        *string `json:"company"`
	Email          string  `json:"email"`
	Whatsapp       string  `json:"whatsapp"`
	Service        *string `json:"service"`
	StartTimeframe *string `json:"startTimeframe"`
	Message        string  `json:"message"`
	Website        string  `json:"website"`
}

func (in *LeadCreateInput) Validate() (*domain.Lead, error) {
	l := &domain.Lead{Name: domain.Trim(in.Name), Company: domain.TrimPtr(in.Company),
		Email: domain.Trim(in.Email), ServiceSlug: domain.TrimPtr(in.Service),
		StartTimeframe: domain.TrimPtr(in.StartTimeframe), Message: domain.Trim(in.Message), Status: domain.LeadNew}
	v := domain.NewValidator()
	v.Str("name", l.Name, true, 120)
	v.OptStr("company", l.Company, 160)
	v.Email("email", l.Email)
	l.Whatsapp = phone(v, "whatsapp", in.Whatsapp)
	if l.ServiceSlug != nil && !domain.IsSlug(*l.ServiceSlug) {
		v.Add("service", "Layanan tidak ditemukan.")
	}
	v.OptStr("startTimeframe", l.StartTimeframe, 100)
	v.Str("message", l.Message, true, 3000)
	if !v.Has("message") {
		v.MinLen("message", l.Message, 10)
	}
	return l, v.Err()
}

type LeadPatchInput struct {
	Status httpx.Optional[string] `json:"status"`
	Notes  httpx.Optional[string] `json:"notes"`
}

func (in *LeadPatchInput) Validate() (status *string, setNotes bool, notes *string, err error) {
	if !in.Status.Set && !in.Notes.Set {
		return nil, false, nil, &domain.ValidationError{Message: "Kirim minimal satu perubahan.", Fields: map[string]string{}}
	}
	v := domain.NewValidator()
	if in.Status.Set {
		s := domain.Trim(in.Status.Value)
		if in.Status.Null {
			v.Add("status", domain.MsgRequired)
		} else {
			v.Enum("status", s, domain.LeadNew, domain.LeadContacted, domain.LeadDone)
			status = &s
		}
	}
	if in.Notes.Set {
		setNotes = true
		if !in.Notes.Null {
			notes = domain.TrimPtr(&in.Notes.Value)
			v.OptStr("notes", notes, 5000)
		}
	}
	return status, setNotes, notes, v.Err()
}

// ---- auth & users ----

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserCreateInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

func (in *UserCreateInput) Validate() error {
	in.Name, in.Email, in.Role = domain.Trim(in.Name), domain.Trim(in.Email), domain.Trim(in.Role)
	v := domain.NewValidator()
	v.Str("name", in.Name, true, 120)
	v.Email("email", in.Email)
	v.Enum("role", in.Role, domain.RoleAdmin, domain.RoleEditor)
	if msg := auth.PasswordProblem(in.Password); msg != "" {
		v.Add("password", msg)
	}
	return v.Err()
}

type UserUpdateInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"isActive"`
}

func (in *UserUpdateInput) Validate() error {
	in.Name, in.Email, in.Role = domain.Trim(in.Name), domain.Trim(in.Email), domain.Trim(in.Role)
	v := domain.NewValidator()
	v.Str("name", in.Name, true, 120)
	v.Email("email", in.Email)
	v.Enum("role", in.Role, domain.RoleAdmin, domain.RoleEditor)
	return v.Err()
}

type PasswordResetInput struct {
	Password string `json:"password"`
}

func (in *PasswordResetInput) Validate() error {
	v := domain.NewValidator()
	if msg := auth.PasswordProblem(in.Password); msg != "" {
		v.Add("password", msg)
	}
	return v.Err()
}
