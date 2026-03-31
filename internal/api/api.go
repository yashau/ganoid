package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yashau/ganoid/internal/config"
	"github.com/yashau/ganoid/internal/manager"
)

// Server holds the HTTP router and dependencies.
type Server struct {
	router  *chi.Mux
	cfg     *config.Config
	mgr     *manager.Manager
	version string
}

// New creates and returns a configured HTTP server.
func New(cfg *config.Config, mgr *manager.Manager, uiFS http.FileSystem, version string) *Server {
	s := &Server{cfg: cfg, mgr: mgr, version: version}
	s.router = chi.NewRouter()
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(30 * time.Second))

	// API routes — all require a valid Bearer token
	s.router.Route("/api", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/status", s.handleStatus)
		r.Get("/profiles", s.handleListProfiles)
		r.Post("/profiles", s.handleCreateProfile)
		r.Put("/profiles/{id}", s.handleUpdateProfile)
		r.Delete("/profiles/{id}", s.handleDeleteProfile)
		r.Post("/profiles/{id}/switch", s.handleSwitchProfile)
		r.Get("/tailscale/status", s.handleTailscaleStatus)
	})

	// Serve embedded UI for all other routes.
	// Unknown paths fall back to index.html so SvelteKit's client-side router
	// handles them (direct navigation to /profiles, page refresh, etc.).
	if uiFS != nil {
		s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			rctx := chi.RouteContext(r.Context())
			pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
			// Check if the file exists; if not, serve index.html.
			filePath := strings.TrimPrefix(r.URL.Path, pathPrefix)
			if filePath == "" {
				filePath = "/"
			}
			f, err := uiFS.Open(filePath)
			if err != nil {
				// Not a real file — let the SPA handle the route.
				r2 := r.Clone(r.Context())
				r2.URL.Path = pathPrefix + "/index.html"
				http.StripPrefix(pathPrefix, http.FileServer(uiFS)).ServeHTTP(w, r2)
				return
			}
			f.Close()
			http.StripPrefix(pathPrefix, http.FileServer(uiFS)).ServeHTTP(w, r)
		})
	} else {
		s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintln(w, "Ganoid — UI not embedded (dev mode)")
		})
	}

	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

// --- handlers ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	active, _ := s.cfg.ActiveProfile()

	ctx := r.Context()
	tsStatus, err := s.mgr.TailscaleStatus(ctx)

	resp := map[string]interface{}{
		"active_profile": active,
		"version":        s.version,
		"tailscale": map[string]interface{}{
			"backend_state": manager.BackendState(tsStatus),
			"peer_count":    manager.PeerCount(tsStatus),
		},
	}
	if err != nil {
		resp["tailscale_error"] = err.Error()
	}
	jsonOK(w, resp)
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	store := s.cfg.GetStore()
	jsonOK(w, store)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		LoginServer string `json:"login_server"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" || req.Name == "" {
		jsonError(w, http.StatusBadRequest, "id and name are required")
		return
	}

	p := config.Profile{
		ID:          req.ID,
		Name:        req.Name,
		LoginServer: req.LoginServer,
		CreatedAt:   time.Now().UTC(),
		LastUsed:    time.Now().UTC(),
	}
	if err := s.cfg.AddProfile(p); err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, p)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string `json:"name"`
		LoginServer string `json:"login_server"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.cfg.UpdateProfile(id, req.Name, req.LoginServer); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	p, _ := s.cfg.GetProfile(id)
	jsonOK(w, p)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.cfg.DeleteProfile(id); err != nil {
		status := http.StatusNotFound
		if err.Error() == "cannot delete the active profile" {
			status = http.StatusConflict
		}
		jsonError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSwitchProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Flush SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	events := s.mgr.SwitchProfile(r.Context(), id)
	for ev := range events {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if canFlush {
			flusher.Flush()
		}
	}
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.mgr.TailscaleStatus(r.Context())
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	jsonOK(w, status)
}

// --- middleware ---

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := s.cfg.AuthToken()
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			jsonError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
