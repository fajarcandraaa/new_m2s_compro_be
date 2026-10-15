package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) error {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toSite(st))
	return nil
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) error {
	audience, err := httpx.QueryEnum(r, "audience", "business", "personal")
	if err != nil {
		return err
	}
	list, err := s.store.ListServices(r.Context(), store.ServiceFilter{PublishedOnly: true, Audience: audience})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toService), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) error {
	slug, err := httpx.PathSlug(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetPublishedServiceBySlug(r.Context(), slug)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toService(v))
	return nil
}

func (s *Server) listCaseStudies(w http.ResponseWriter, r *http.Request) error {
	featured, err := httpx.QueryBool(r, "featured")
	if err != nil {
		return err
	}
	category, err := httpx.QueryString(r, "category", 60)
	if err != nil {
		return err
	}
	list, err := s.store.ListPublicCaseStudies(r.Context(), featured, category)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toCaseStudySummary), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) getCaseStudy(w http.ResponseWriter, r *http.Request) error {
	slug, err := httpx.PathSlug(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetPublishedCaseStudyBySlug(r.Context(), slug)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toCaseStudy(v))
	return nil
}

func (s *Server) listArticles(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	tag, err := httpx.QuerySlug(r, "tag")
	if err != nil {
		return err
	}
	list, total, err := s.store.ListPublicArticles(r.Context(), tag, store.Page{Page: page, Size: size})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toArticleSummary), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) getArticle(w http.ResponseWriter, r *http.Request) error {
	slug, err := httpx.PathSlug(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetVisibleArticleBySlug(r.Context(), slug)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toArticle(v))
	return nil
}

func (s *Server) listRelatedArticles(w http.ResponseWriter, r *http.Request) error {
	slug, err := httpx.PathSlug(r)
	if err != nil {
		return err
	}
	limit, err := httpx.QueryInt(r, "limit", 1, 12, 3)
	if err != nil {
		return err
	}
	a, err := s.store.GetVisibleArticleBySlug(r.Context(), slug)
	if err != nil {
		return err
	}
	list, err := s.store.ListRelatedArticles(r.Context(), a.ID, limit)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toArticleSummary), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListTags(r.Context())
	if err != nil {
		return err
	}
	out := make([]tagWithCountDTO, len(list))
	for i, t := range list {
		out[i] = tagWithCountDTO{tagDTO: tagDTO{Slug: t.Slug, Name: t.Name}, ArticleCount: t.ArticleCount}
	}
	httpx.List(w, out, httpx.Unpaginated(len(out)))
	return nil
}

func (s *Server) listPromos(w http.ResponseWriter, r *http.Request) error {
	status, err := httpx.QueryEnum(r, "status", domain.PromoActive, domain.PromoUpcoming, domain.PromoEnded)
	if err != nil {
		return err
	}
	list, err := s.store.ListPublicPromos(r.Context(), status)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toPromo), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) listTestimonials(w http.ResponseWriter, r *http.Request) error {
	limit, err := httpx.QueryInt(r, "limit", 1, 50, 0)
	if err != nil {
		return err
	}
	active := true
	list, err := s.store.ListTestimonials(r.Context(), &active, limit)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toTestimonial), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) listClientLogos(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListClientLogos(r.Context())
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toClientLogo), httpx.Unpaginated(len(list)))
	return nil
}

// createLead handles the /diskusi form: rate limit (5/min per client IP)
// → decode → honeypot (fake 201, nothing stored) → validation → insert.
func (s *Server) createLead(w http.ResponseWriter, r *http.Request) error {
	ip := httpx.IPFromContext(r.Context())
	if ok, retry := s.leadLimiter.Allow(ip); !ok {
		return httpx.RateLimited(retry)
	}
	var in LeadCreateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Website) != "" {
		httpx.Data(w, http.StatusCreated, leadCreatedDTO{ID: randomUUID(), CreatedAt: httpx.Time(s.now())})
		return nil
	}
	lead, err := in.Validate()
	if err != nil {
		return err
	}
	if lead.ServiceSlug != nil {
		ok, err := s.store.PublishedServiceExists(r.Context(), *lead.ServiceSlug)
		if err != nil {
			return err
		}
		if !ok {
			return &domain.ValidationError{Message: domain.MsgGeneric, Fields: map[string]string{"service": "Layanan tidak ditemukan."}}
		}
	}
	if ip != "" {
		mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
		mac.Write([]byte(ip))
		h := hex.EncodeToString(mac.Sum(nil))
		lead.IPHash = &h
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		if utf8.RuneCountInString(ua) > 300 {
			ua = string([]rune(ua)[:300])
		}
		lead.UserAgent = &ua
	}
	if err := s.store.CreateLead(r.Context(), lead); err != nil {
		return err
	}
	httpx.Data(w, http.StatusCreated, leadCreatedDTO{ID: lead.ID, CreatedAt: httpx.Time(lead.CreatedAt)})
	return nil
}

// randomUUID returns a random (v4) UUID string.
func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
