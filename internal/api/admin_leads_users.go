package api

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// wib is Asia/Jakarta (UTC+7, no DST); used for the export file name.
var wib = time.FixedZone("WIB", 7*3600)

// ---- leads ----

func leadFilter(r *http.Request) (store.LeadFilter, error) {
	q, err := httpx.QueryString(r, "q", 100)
	if err != nil {
		return store.LeadFilter{}, err
	}
	status, err := httpx.QueryEnum(r, "status", domain.LeadNew, domain.LeadContacted, domain.LeadDone)
	if err != nil {
		return store.LeadFilter{}, err
	}
	return store.LeadFilter{Q: q, Status: status}, nil
}

func (s *Server) adminListLeads(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	f, err := leadFilter(r)
	if err != nil {
		return err
	}
	f.Page = &store.Page{Page: page, Size: size}
	list, total, err := s.store.ListLeads(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toLead), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) adminGetLead(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	v, err := s.store.GetLead(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toLead(v))
	return nil
}

func (s *Server) adminPatchLead(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	var in LeadPatchInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	status, setNotes, notes, err := in.Validate()
	if err != nil {
		return err
	}
	v, err := s.store.PatchLead(r.Context(), id, status, setNotes, notes)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toLead(v))
	return nil
}

// csvSafe guards against CSV/formula injection.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// LeadsCSV renders leads as UTF-8 CSV with BOM, CRLF and RFC 4180 quoting.
func LeadsCSV(leads []domain.Lead) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM for Excel
	cw := csv.NewWriter(&buf)
	cw.UseCRLF = true
	header := []string{"createdAt", "name", "company", "email", "whatsapp", "service", "startTimeframe", "status", "message", "notes"}
	if err := cw.Write(header); err != nil {
		return nil, err
	}
	for _, l := range leads {
		service := deref(l.ServiceTitle)
		if service == "" {
			service = deref(l.ServiceSlug)
		}
		row := []string{l.CreatedAt.UTC().Format(time.RFC3339), l.Name, deref(l.Company), l.Email, l.Whatsapp,
			service, deref(l.StartTimeframe), l.Status, l.Message, deref(l.Notes)}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if err := cw.Write(row); err != nil {
			return nil, err
		}
	}
	cw.Flush()
	return buf.Bytes(), cw.Error()
}

func (s *Server) adminExportLeads(w http.ResponseWriter, r *http.Request) error {
	f, err := leadFilter(r)
	if err != nil {
		return err
	}
	list, _, err := s.store.ListLeads(r.Context(), f)
	if err != nil {
		return err
	}
	body, err := LeadsCSV(list)
	if err != nil {
		return err
	}
	name := "leads-" + s.now().In(wib).Format("20060102") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

// ---- users ----

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	page, size, err := httpx.Page(r)
	if err != nil {
		return err
	}
	q, err := httpx.QueryString(r, "q", 100)
	if err != nil {
		return err
	}
	list, total, err := s.store.ListUsers(r.Context(), q, store.Page{Page: page, Size: size})
	if err != nil {
		return err
	}
	httpx.List(w, mapSlice(list, toUser), httpx.PageMeta{Page: page, PageSize: size, Total: total})
	return nil
}

func (s *Server) adminGetUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	u, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toUser(u))
	return nil
}

func emailTaken() error {
	return &domain.ConflictError{Code: "conflict", Message: "Email sudah dipakai.", Fields: map[string]string{"email": "Email sudah dipakai."}}
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) error {
	var in UserCreateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	if err := in.Validate(); err != nil {
		return err
	}
	taken, err := s.store.EmailTaken(r.Context(), in.Email, "")
	if err != nil {
		return err
	}
	if taken {
		return emailTaken()
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u, err := s.store.CreateUser(r.Context(), in.Name, in.Email, in.Role, hash)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusCreated, toUser(u))
	return nil
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	var in UserUpdateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	if err := in.Validate(); err != nil {
		return err
	}
	current, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		return err
	}
	me := auth.UserFrom(r.Context())
	if me.ID == id {
		if in.Role != current.Role {
			msg := "Anda tidak dapat mengubah peran akun Anda sendiri."
			return &domain.ConflictError{Code: "conflict", Message: msg, Fields: map[string]string{"role": msg}}
		}
		if !in.IsActive {
			msg := "Anda tidak dapat menonaktifkan akun Anda sendiri."
			return &domain.ConflictError{Code: "conflict", Message: msg, Fields: map[string]string{"isActive": msg}}
		}
	}
	taken, err := s.store.EmailTaken(r.Context(), in.Email, id)
	if err != nil {
		return err
	}
	if taken {
		return emailTaken()
	}
	bump := in.Role != current.Role || (current.IsActive && !in.IsActive)
	u, err := s.store.UpdateUser(r.Context(), id, in.Name, in.Email, in.Role, in.IsActive, bump)
	if err != nil {
		return err
	}
	httpx.Data(w, http.StatusOK, toUser(u))
	return nil
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	if auth.UserFrom(r.Context()).ID == id {
		return &domain.ConflictError{Code: "conflict", Message: "Anda tidak dapat menghapus akun Anda sendiri."}
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (s *Server) adminResetPassword(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r)
	if err != nil {
		return err
	}
	var in PasswordResetInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	if err := in.Validate(); err != nil {
		return err
	}
	if _, err := s.store.GetUser(r.Context(), id); err != nil {
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u, err := s.store.SetPassword(r.Context(), id, hash)
	if err != nil {
		return err
	}
	// Resetting your own password revokes your session too; re-issue the
	// cookie so the admin stays logged in.
	if auth.UserFrom(r.Context()).ID == id {
		if err := s.auth.SetCookie(w, u); err != nil {
			return err
		}
	}
	httpx.NoContent(w)
	return nil
}
