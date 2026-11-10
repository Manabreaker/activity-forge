package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/render"
)

type Options struct {
	View  render.View
	Save  func(context.Context, forge.Plan) (string, error)
	Fetch func(context.Context, forge.Config) (forge.Snapshot, error)
	Push  func(context.Context) error
}

type Server struct {
	mu       sync.Mutex
	view     render.View
	opts     Options
	token    string
	revision int64
}

func New(opts Options) (*Server, error) {
	if opts.View.Kind == "" {
		opts.View.Kind = "preview"
	}
	if opts.View.Kind == "preview" {
		if err := forge.ValidatePlan(opts.View.Plan); err != nil {
			return nil, err
		}
	}
	if len(opts.View.Plan.Days) == 0 {
		return nil, fmt.Errorf("cannot preview an empty calendar")
	}
	if opts.View.Kind != "preview" {
		opts.View.Frozen = true
	}
	opts.View.CanFetch = opts.Fetch != nil
	if opts.View.Kind == "publish" && (opts.View.Verification == nil || !opts.View.Verification.OK) {
		opts.View.CanPush = false
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &Server{opts: opts, view: opts.View, token: hex.EncodeToString(key), revision: opts.View.Revision}, nil
}

func (s *Server) Token() string { return s.token }

func (s *Server) Handler(host string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/export", s.export)
	mux.HandleFunc("POST /api/preview", s.preview)
	mux.HandleFunc("POST /api/freeze", s.freeze)
	mux.HandleFunc("POST /api/draft", s.draft)
	mux.HandleFunc("POST /api/fetch", s.fetch)
	mux.HandleFunc("POST /api/push", s.push)
	mux.HandleFunc("POST /api/cancel", s.cancel)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+s.token+"'; style-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		if r.Host != host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+host) {
			replyError(w, 403, "local session host/origin check failed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-ActivityForge-Token")), []byte(s.token)) != 1 {
			replyError(w, 403, "local session token required")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func replyError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err == nil {
		err = forge.Decode(data, v)
	}
	if err != nil {
		replyError(w, 400, err.Error())
		return false
	}
	return true
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.Page(w, s.view, false, s.token); err != nil {
		replyError(w, 500, err.Error())
	}
}
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply(w, s.view)
}

func (s *Server) editable(w http.ResponseWriter) bool {
	if s.view.Kind != "preview" || s.view.Frozen {
		replyError(w, 409, "this plan is immutable; start a new draft before editing")
		return false
	}
	return true
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config      forge.Config `json:"config"`
		Revision    int64        `json:"revision"`
		ApplyPreset bool         `json:"apply_preset"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.editable(w) {
		return
	}
	if req.Revision <= s.revision {
		replyError(w, 409, "stale preview update")
		return
	}
	c := req.Config
	if req.ApplyPreset {
		c = forge.ApplyPreset(c, c.Preset)
	}
	p, err := forge.Generate(c, s.view.Plan.Existing)
	if err != nil {
		replyError(w, 400, err.Error())
		return
	}
	s.revision = req.Revision
	s.view.Revision = req.Revision
	s.view.Plan = p
	s.view.SavedPath = ""
	s.view.Notice = ""
	reply(w, s.view)
}

func (s *Server) freeze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.editable(w) {
		return
	}
	if req.ID != s.view.Plan.ID {
		replyError(w, 409, "preview changed; review the current plan before saving")
		return
	}
	if s.opts.Save == nil {
		replyError(w, 409, "no plan destination configured")
		return
	}
	path, err := s.opts.Save(r.Context(), s.view.Plan)
	if err != nil {
		replyError(w, 500, err.Error())
		return
	}
	s.view.Frozen = true
	s.view.SavedPath = path
	reply(w, s.view)
}

func (s *Server) draft(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Kind == "publish" && !s.view.Published {
		plan, err := forge.Generate(s.view.Plan.Config, s.view.Plan.Existing)
		if err != nil {
			replyError(w, 400, err.Error())
			return
		}
		s.view = render.View{Plan: plan, Kind: "preview", Revision: s.revision, CanFetch: s.opts.Fetch != nil, Notice: "New draft. Existing Git history is unchanged. Save a new plan and generate it in a NEW empty --repo directory; the old publication approval has been cancelled."}
		reply(w, s.view)
		return
	}
	if s.view.Kind != "preview" {
		replyError(w, 409, "local Git history cannot be regenerated in this view; use a new explicit plan and a separate empty repository")
		return
	}
	s.view.Frozen = false
	s.view.SavedPath = ""
	s.view.Notice = "New draft. Previously saved plans remain unchanged."
	reply(w, s.view)
}

func (s *Server) fetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config *forge.Config `json:"config"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.editable(w) {
		return
	}
	if s.opts.Fetch == nil {
		replyError(w, 409, "GitHub CLI integration is not configured")
		return
	}
	c := s.view.Plan.Config
	if req.Config != nil {
		c = *req.Config
	}
	if err := c.Validate(); err != nil {
		replyError(w, 400, err.Error())
		return
	}
	data, err := s.opts.Fetch(r.Context(), c)
	if err != nil {
		replyError(w, 400, err.Error())
		return
	}
	p, err := forge.Generate(c, data)
	if err != nil {
		replyError(w, 400, err.Error())
		return
	}
	s.view.Plan = p
	s.view.Notice = "Current GitHub contributions loaded and cached. Only additional commits are planned."
	reply(w, s.view)
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	types := map[string]string{"json": "application/json", "csv": "text/csv; charset=utf-8", "svg": "image/svg+xml", "png": "image/png", "html": "text/html; charset=utf-8"}
	contentType, ok := types[format]
	if !ok {
		replyError(w, 400, "unsupported export format")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="activityforge-%s.%s"`, s.view.Plan.ID[:12], format))
	if err := render.Export(w, s.view, format); err != nil {
		replyError(w, 500, err.Error())
	}
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Head string `json:"head"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Kind != "publish" || !s.view.CanPush || s.view.Published || s.opts.Push == nil || s.view.Verification == nil || !s.view.Verification.OK {
		replyError(w, 409, "publication is not available; prepare and verify again")
		return
	}
	if req.ID != s.view.Plan.ID || req.Head != s.view.Verification.Head {
		replyError(w, 409, "approval does not match the verified plan and HEAD")
		return
	}
	s.view.CanPush = false
	if err := s.opts.Push(r.Context()); err != nil {
		s.view.Notice = "Publication failed. Re-run publish to verify local and remote state before another approval."
		replyError(w, 400, err.Error())
		return
	}
	s.view.Published = true
	s.view.Notice = "Published with a normal push. On your GitHub profile, open Contribution settings → Private contributions to include private activity. Indexing can take up to 24 hours."
	reply(w, s.view)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Kind != "publish" || s.view.Published {
		replyError(w, 409, "no pending publication")
		return
	}
	s.view.CanPush = false
	s.view.Notice = "Publication cancelled. You can export this preview or close the page."
	reply(w, s.view)
}
