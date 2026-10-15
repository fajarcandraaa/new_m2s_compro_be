// Package api wires the chi router and the HTTP handlers.
package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/config"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/media"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/ratelimit"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/revalidate"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// Deps are the router dependencies.
type Deps struct {
	Store       *store.Store
	Config      *config.Config
	Revalidator revalidate.Firer
	// Optional overrides (tests); defaults: 5/min leads, 10/min login.
	LeadLimiter  *ratelimit.Limiter
	LoginLimiter *ratelimit.Limiter
	Now          func() time.Time
}

// Server holds the handler dependencies.
type Server struct {
	store        *store.Store
	cfg          *config.Config
	auth         *auth.Manager
	reval        revalidate.Firer
	leadLimiter  *ratelimit.Limiter
	loginLimiter *ratelimit.Limiter
	now          func() time.Time
}

// NewRouter builds the HTTP handler.
func NewRouter(d Deps) http.Handler {
	s := &Server{
		store: d.Store,
		cfg:   d.Config,
		auth: &auth.Manager{
			Secret: []byte(d.Config.JWTSecret), TTL: d.Config.JWTTTL,
			CookieSecure: d.Config.CookieSecure, Users: d.Store,
		},
		reval:        d.Revalidator,
		leadLimiter:  d.LeadLimiter,
		loginLimiter: d.LoginLimiter,
		now:          d.Now,
	}
	if s.reval == nil {
		s.reval = revalidate.New("", "")
	}
	if s.leadLimiter == nil {
		s.leadLimiter = ratelimit.New(5, time.Minute)
	}
	if s.loginLimiter == nil {
		s.loginLimiter = ratelimit.New(10, time.Minute)
	}
	if s.now == nil {
		s.now = time.Now
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(httpx.ClientIPMiddleware(d.Config.TrustedProxyHops))
	r.Use(httpx.Recoverer)
	r.Use(httpx.AccessLog)
	r.Use(middleware.Timeout(15 * time.Second))
	r.NotFound(httpx.JSONNotFound)
	r.MethodNotAllowed(httpx.JSONMethodNotAllowed)

	r.Get("/healthz", s.health)
	r.Method(http.MethodGet, "/uploads/*", media.FileServer(d.Config.UploadDir))
	r.Method(http.MethodHead, "/uploads/*", media.FileServer(d.Config.UploadDir))

	h := httpx.H
	r.Route("/api/v1", func(r chi.Router) {
		r.NotFound(httpx.JSONNotFound)
		r.MethodNotAllowed(httpx.JSONMethodNotAllowed)

		// Public.
		r.Get("/site", h(s.getSite))
		r.Get("/services", h(s.listServices))
		r.Get("/services/{slug}", h(s.getService))
		r.Get("/case-studies", h(s.listCaseStudies))
		r.Get("/case-studies/{slug}", h(s.getCaseStudy))
		r.Get("/articles", h(s.listArticles))
		r.Get("/articles/{slug}", h(s.getArticle))
		r.Get("/articles/{slug}/related", h(s.listRelatedArticles))
		r.Get("/tags", h(s.listTags))
		r.Get("/promos", h(s.listPromos))
		r.Get("/testimonials", h(s.listTestimonials))
		r.Get("/client-logos", h(s.listClientLogos))
		r.Post("/leads", h(s.createLead))

		// Auth (CSRF before anything else).
		r.Route("/auth", func(r chi.Router) {
			r.Use(httpx.NoStore)
			r.Use(auth.CSRF)
			r.Post("/login", h(s.login))
			r.Post("/logout", h(s.logout))
			r.With(s.auth.RequireSession).Get("/me", h(s.me))
		})

		// Admin: CSRF → session → role.
		r.Route("/admin", func(r chi.Router) {
			r.Use(httpx.NoStore)
			r.Use(auth.CSRF)
			r.Use(s.auth.RequireSession)
			adminOnly := auth.RequireRole(domain.RoleAdmin)
			r.Use(auth.RequireRole(domain.RoleAdmin, domain.RoleEditor))

			r.Get("/dashboard", h(s.adminDashboard))

			r.Get("/settings", h(s.adminGetSettings))
			r.With(adminOnly).Put("/settings", h(s.adminUpdateSettings))
			r.Patch("/settings/toggles", h(s.adminUpdateToggles))

			r.Get("/services", h(s.adminListServices))
			r.Post("/services", h(s.adminCreateService))
			r.Put("/services/order", h(s.adminReorderServices))
			r.Get("/services/{id}", h(s.adminGetService))
			r.Put("/services/{id}", h(s.adminUpdateService))
			r.Delete("/services/{id}", h(s.adminDeleteService))

			r.Get("/case-studies", h(s.adminListCaseStudies))
			r.Post("/case-studies", h(s.adminCreateCaseStudy))
			r.Get("/case-studies/{id}", h(s.adminGetCaseStudy))
			r.Put("/case-studies/{id}", h(s.adminUpdateCaseStudy))
			r.Delete("/case-studies/{id}", h(s.adminDeleteCaseStudy))

			r.Get("/articles", h(s.adminListArticles))
			r.Post("/articles", h(s.adminCreateArticle))
			r.Get("/articles/{id}", h(s.adminGetArticle))
			r.Put("/articles/{id}", h(s.adminUpdateArticle))
			r.Delete("/articles/{id}", h(s.adminDeleteArticle))

			r.Get("/promos", h(s.adminListPromos))
			r.Post("/promos", h(s.adminCreatePromo))
			r.Get("/promos/{id}", h(s.adminGetPromo))
			r.Put("/promos/{id}", h(s.adminUpdatePromo))
			r.Delete("/promos/{id}", h(s.adminDeletePromo))

			r.Get("/testimonials", h(s.adminListTestimonials))
			r.Post("/testimonials", h(s.adminCreateTestimonial))
			r.Put("/testimonials/order", h(s.adminReorderTestimonials))
			r.Get("/testimonials/{id}", h(s.adminGetTestimonial))
			r.Put("/testimonials/{id}", h(s.adminUpdateTestimonial))
			r.Delete("/testimonials/{id}", h(s.adminDeleteTestimonial))

			r.Get("/client-logos", h(s.adminListClientLogos))
			r.Post("/client-logos", h(s.adminCreateClientLogo))
			r.Put("/client-logos/order", h(s.adminReorderClientLogos))
			r.Get("/client-logos/{id}", h(s.adminGetClientLogo))
			r.Put("/client-logos/{id}", h(s.adminUpdateClientLogo))
			r.Delete("/client-logos/{id}", h(s.adminDeleteClientLogo))

			r.Get("/leads", h(s.adminListLeads))
			r.With(adminOnly).Get("/leads/export", h(s.adminExportLeads))
			r.Get("/leads/{id}", h(s.adminGetLead))
			r.With(adminOnly).Patch("/leads/{id}", h(s.adminPatchLead))

			r.Post("/media", h(s.adminUploadMedia))

			r.Group(func(r chi.Router) {
				r.Use(adminOnly)
				r.Get("/users", h(s.adminListUsers))
				r.Post("/users", h(s.adminCreateUser))
				r.Get("/users/{id}", h(s.adminGetUser))
				r.Put("/users/{id}", h(s.adminUpdateUser))
				r.Delete("/users/{id}", h(s.adminDeleteUser))
				r.Post("/users/{id}/password", h(s.adminResetPassword))
			})
		})
	})
	return r
}

type healthDTO struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, healthDTO{Status: "degraded", DB: "down"})
		return
	}
	httpx.JSON(w, http.StatusOK, healthDTO{Status: "ok", DB: "ok"})
}
