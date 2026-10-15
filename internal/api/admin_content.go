package api

import (
	"net/http"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/media"
	rv "github.com/fajarcandraaa/new_m2s_compro_be/internal/revalidate"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// ---- dashboard ----

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) error {
	d, err := s.store.Dashboard(r.Context())
	if err != nil {
		return err
	}
	var out dashboardDTO
	out.Counts.NewLeads = d.NewLeads
	out.Counts.DraftContent = d.DraftContent
	out.Counts.ActivePromos = d.ActivePromos
	out.LatestLeads = mapSlice(d.LatestLeads, toLead)
	out.RecentDrafts = mapSlice(d.RecentDrafts, func(it *domain.DraftItem) draftItemDTO {
		return draftItemDTO{Type: it.Type, ID: it.ID, Title: it.Title, UpdatedAt: httpx.Time(it.UpdatedAt)}
	})
	httpx.Data(w, http.StatusOK, out)
	return nil
}

// ---- settings ----

func (s *Server) adminGetSettings(w http.ResponseWriter, r *http.Request) error {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toSettingsAdmin(st))
	return nil
}

func (s *Server) adminUpdateSettings(w http.ResponseWriter, r *http.Request) error {
	var in SettingsInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate()
	if err != nil {
		return err
	}
	st, err := s.store.UpdateSettings(r.Context(), v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagSite)
	httpx.Data(w, http.StatusOK, toSettingsAdmin(st))
	return nil
}

func (s *Server) adminUpdateToggles(w http.ResponseWriter, r *http.Request) error {
	var in TogglesInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	showT, showL, err := in.Validate()
	if err != nil {
		return err
	}
	st, err := s.store.UpdateToggles(r.Context(), showT, showL)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagSite)
	httpx.Data(w, http.StatusOK, toSettingsAdmin(st))
	return nil
}

// ---- services ----

func (s *Server) adminListServices(w http.ResponseWriter, r *http.Request) error {
	status, err := httpx.QueryEnum(r, "status", domain.StatusDraft, domain.StatusPublished)
	if err != nil {
		return err
	}
	audience, err := httpx.QueryEnum(r, "audience", "business", "personal")
	if err != nil {
		return err
	}
	list, err := s.store.ListServices(r.Context(), store.ServiceFilter{Status: status, Audience: audience})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toServiceAdmin), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) adminGetService(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetService(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toServiceAdmin(v))
	return nil
}

func (s *Server) saveService(w http.ResponseWriter, r *http.Request, id string) error {
	var in ServiceInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate(id == "")
	if err != nil {
		return err
	}
	saved, err := s.store.SaveService(r.Context(), id, v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagServices, rv.TagPromos)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toServiceAdmin(saved))
	return nil
}

func (s *Server) adminCreateService(w http.ResponseWriter, r *http.Request) error {
	return s.saveService(w, r, "")
}

func (s *Server) adminUpdateService(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.saveService(w, r, id)
}

func (s *Server) adminDeleteService(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteService(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagServices, rv.TagPromos)
	httpx.NoContent(w)
	return nil
}

func (s *Server) adminReorderServices(w http.ResponseWriter, r *http.Request) error {
	var in ReorderInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	ids, err := in.Validate()
	if err != nil {
		return err
	}
	if err := s.store.ReorderServices(r.Context(), ids); err != nil {
		return err
	}
	s.reval.Fire(rv.TagServices, rv.TagPromos)
	list, err := s.store.ListServices(r.Context(), store.ServiceFilter{})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toServiceAdmin), httpx.Unpaginated(len(list)))
	return nil
}

// ---- case studies ----

func (s *Server) adminListCaseStudies(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	q, err := httpx.QueryString(r, "q", 100)
	if err != nil {
		return err
	}
	status, err := httpx.QueryEnum(r, "status", domain.StatusDraft, domain.StatusPublished)
	if err != nil {
		return err
	}
	list, total, err := s.store.ListAdminCaseStudies(r.Context(), store.AdminCaseStudyFilter{
		Q: q, Status: status, Page: store.Page{Page: page, Size: size}})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toCaseStudyAdmin), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) adminGetCaseStudy(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetCaseStudy(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toCaseStudyAdmin(v))
	return nil
}

func (s *Server) saveCaseStudy(w http.ResponseWriter, r *http.Request, id string) error {
	var in CaseStudyInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate(id == "")
	if err != nil {
		return err
	}
	saved, err := s.store.SaveCaseStudy(r.Context(), id, v, in.SortOrder)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagCaseStudies)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toCaseStudyAdmin(saved))
	return nil
}

func (s *Server) adminCreateCaseStudy(w http.ResponseWriter, r *http.Request) error {
	return s.saveCaseStudy(w, r, "")
}

func (s *Server) adminUpdateCaseStudy(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.saveCaseStudy(w, r, id)
}

func (s *Server) adminDeleteCaseStudy(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteCaseStudy(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagCaseStudies)
	httpx.NoContent(w)
	return nil
}

// ---- articles ----

func (s *Server) adminListArticles(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	q, err := httpx.QueryString(r, "q", 100)
	if err != nil {
		return err
	}
	status, err := httpx.QueryEnum(r, "status", "draft", "published", "scheduled")
	if err != nil {
		return err
	}
	tag, err := httpx.QueryString(r, "tag", 120)
	if err != nil {
		return err
	}
	list, total, err := s.store.ListAdminArticles(r.Context(), store.AdminArticleFilter{
		Q: q, Status: status, TagSlug: tag, Page: store.Page{Page: page, Size: size}})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toArticleAdminSummary), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) adminGetArticle(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetArticle(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toArticleAdmin(v))
	return nil
}

func (s *Server) saveArticle(w http.ResponseWriter, r *http.Request, id string) error {
	var in ArticleInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate(id == "", s.now())
	if err != nil {
		return err
	}
	saved, err := s.store.SaveArticle(r.Context(), id, v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagArticles)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toArticleAdmin(saved))
	return nil
}

func (s *Server) adminCreateArticle(w http.ResponseWriter, r *http.Request) error {
	return s.saveArticle(w, r, "")
}

func (s *Server) adminUpdateArticle(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.saveArticle(w, r, id)
}

func (s *Server) adminDeleteArticle(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteArticle(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagArticles)
	httpx.NoContent(w)
	return nil
}

// ---- promos ----

func (s *Server) adminListPromos(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	q, err := httpx.QueryString(r, "q", 100)
	if err != nil {
		return err
	}
	status, err := httpx.QueryEnum(r, "status", domain.PromoActive, domain.PromoUpcoming, domain.PromoEnded)
	if err != nil {
		return err
	}
	list, total, err := s.store.ListAdminPromos(r.Context(), store.AdminPromoFilter{
		Q: q, Status: status, Page: store.Page{Page: page, Size: size}})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toPromoAdmin), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) adminGetPromo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetPromo(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toPromoAdmin(v))
	return nil
}

func (s *Server) savePromo(w http.ResponseWriter, r *http.Request, id string) error {
	var in PromoInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate()
	if err != nil {
		return err
	}
	if v.ServiceID != nil {
		ok, err := s.store.ServiceExists(r.Context(), *v.ServiceID)
		if err != nil {
			return err
		}
		if !ok {
			return &domain.ValidationError{Message: domain.MsgGeneric, Fields: map[string]string{"serviceId": "Layanan tidak ditemukan."}}
		}
	}
	saved, err := s.store.SavePromo(r.Context(), id, v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagPromos)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toPromoAdmin(saved))
	return nil
}

func (s *Server) adminCreatePromo(w http.ResponseWriter, r *http.Request) error {
	return s.savePromo(w, r, "")
}

func (s *Server) adminUpdatePromo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.savePromo(w, r, id)
}

func (s *Server) adminDeletePromo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeletePromo(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagPromos)
	httpx.NoContent(w)
	return nil
}

// ---- testimonials ----

func (s *Server) adminListTestimonials(w http.ResponseWriter, r *http.Request) error {
	active, err := httpx.QueryBool(r, "isActive")
	if err != nil {
		return err
	}
	list, err := s.store.ListTestimonials(r.Context(), active, 0)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toTestimonialAdmin), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) adminGetTestimonial(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetTestimonial(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toTestimonialAdmin(v))
	return nil
}

func (s *Server) saveTestimonial(w http.ResponseWriter, r *http.Request, id string) error {
	var in TestimonialInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate()
	if err != nil {
		return err
	}
	saved, err := s.store.SaveTestimonial(r.Context(), id, v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagTestimonials)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toTestimonialAdmin(saved))
	return nil
}

func (s *Server) adminCreateTestimonial(w http.ResponseWriter, r *http.Request) error {
	return s.saveTestimonial(w, r, "")
}

func (s *Server) adminUpdateTestimonial(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.saveTestimonial(w, r, id)
}

func (s *Server) adminDeleteTestimonial(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTestimonial(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagTestimonials)
	httpx.NoContent(w)
	return nil
}

func (s *Server) adminReorderTestimonials(w http.ResponseWriter, r *http.Request) error {
	var in ReorderInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	ids, err := in.Validate()
	if err != nil {
		return err
	}
	if err := s.store.ReorderTestimonials(r.Context(), ids); err != nil {
		return err
	}
	s.reval.Fire(rv.TagTestimonials)
	list, err := s.store.ListTestimonials(r.Context(), nil, 0)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toTestimonialAdmin), httpx.Unpaginated(len(list)))
	return nil
}

// ---- client logos ----

func (s *Server) adminListClientLogos(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListClientLogos(r.Context())
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toClientLogoAdmin), httpx.Unpaginated(len(list)))
	return nil
}

func (s *Server) adminGetClientLogo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetClientLogo(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toClientLogoAdmin(v))
	return nil
}

func (s *Server) saveClientLogo(w http.ResponseWriter, r *http.Request, id string) error {
	var in ClientLogoInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	v, err := in.Validate()
	if err != nil {
		return err
	}
	saved, err := s.store.SaveClientLogo(r.Context(), id, v)
	if err != nil {
		return err
	}
	s.reval.Fire(rv.TagClientLogos)
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	httpx.Data(w, status, toClientLogoAdmin(saved))
	return nil
}

func (s *Server) adminCreateClientLogo(w http.ResponseWriter, r *http.Request) error {
	return s.saveClientLogo(w, r, "")
}

func (s *Server) adminUpdateClientLogo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	return s.saveClientLogo(w, r, id)
}

func (s *Server) adminDeleteClientLogo(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteClientLogo(r.Context(), id); err != nil {
		return err
	}
	s.reval.Fire(rv.TagClientLogos)
	httpx.NoContent(w)
	return nil
}

func (s *Server) adminReorderClientLogos(w http.ResponseWriter, r *http.Request) error {
	var in ReorderInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	ids, err := in.Validate()
	if err != nil {
		return err
	}
	if err := s.store.ReorderClientLogos(r.Context(), ids); err != nil {
		return err
	}
	s.reval.Fire(rv.TagClientLogos)
	list, err := s.store.ListClientLogos(r.Context())
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toClientLogoAdmin), httpx.Unpaginated(len(list)))
	return nil
}

// ---- media ----

func (s *Server) adminUploadMedia(w http.ResponseWriter, r *http.Request) error {
	up, err := media.Save(w, r, s.cfg.UploadDir, s.cfg.UploadMaxBytes)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusCreated, mediaUploadDTO{URL: up.URL, Filename: up.Filename, MimeType: up.MimeType,
		Size: up.Size, Width: up.Width, Height: up.Height})
	return nil
}
