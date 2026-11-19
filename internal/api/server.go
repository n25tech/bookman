// Package api serves the JSON API under /api and the embedded web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
	"github.com/n25tech/bookman/internal/library"
	"github.com/n25tech/bookman/internal/thumbs"
)

const (
	cookieName   = "bookman_session"
	sessionTTL   = 30 * 24 * time.Hour
	maxJSONBody  = 1 << 20
	progressBody = 4 << 10
)

// Server holds the dependencies of the HTTP handlers.
type Server struct {
	DB      *db.DB
	Scanner *library.Scanner
	Thumbs  *thumbs.Worker
	Pages   *formats.PageCache
	Web     fs.FS // built frontend (web/dist)
	// ScanAll is run in the background by POST /api/admin/scan.
	ScanAll func()
}

type ctxKey struct{}

func userFrom(r *http.Request) db.User { return r.Context().Value(ctxKey{}).(db.User) }

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := http.NewServeMux()

	// Public.
	api.HandleFunc("GET /api/auth/status", s.authStatus)
	api.HandleFunc("POST /api/auth/setup", s.setup)
	api.HandleFunc("POST /api/auth/login", s.login)
	api.HandleFunc("POST /api/auth/logout", s.logout)
	api.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Signed in.
	auth := func(pattern string, h http.HandlerFunc) { api.Handle(pattern, s.requireUser(h)) }
	admin := func(pattern string, h http.HandlerFunc) { api.Handle(pattern, s.requireUser(s.requireAdmin(h))) }

	auth("GET /api/auth/me", s.me)
	auth("GET /api/libraries", s.libraries)
	auth("GET /api/libraries/{id}/books", s.libraryBooks)
	auth("GET /api/libraries/{id}/series", s.librarySeries)
	auth("GET /api/series/{id}", s.series)
	auth("GET /api/books/{id}", s.book)
	auth("GET /api/books/{id}/cover", s.cover)
	auth("GET /api/books/{id}/file", s.file)
	auth("GET /api/books/{id}/pages/{n}", s.page)
	auth("GET /api/books/{id}/progress", s.getProgress)
	auth("PUT /api/books/{id}/progress", s.putProgress)
	auth("POST /api/books/{id}/progress", s.putProgress) // sendBeacon always POSTs
	auth("DELETE /api/books/{id}/progress", s.deleteProgress)
	auth("POST /api/books/{id}/finished", s.finished)
	auth("GET /api/books/{id}/sessions", s.sessions)
	auth("GET /api/books/{id}/notes", s.notes)
	auth("POST /api/books/{id}/notes", s.createNote)
	auth("GET /api/books/{id}/notes/export.md", s.exportNotes)
	auth("PATCH /api/notes/{id}", s.updateNote)
	auth("DELETE /api/notes/{id}", s.deleteNote)
	auth("GET /api/continue-reading", s.continueReading)
	auth("GET /api/recently-added", s.recentlyAdded)
	admin("POST /api/admin/scan", s.adminScan)
	admin("GET /api/admin/scan", s.adminScanStatus)
	admin("GET /api/admin/users", s.adminUsers)
	admin("POST /api/admin/users", s.adminCreateUser)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "no such endpoint") })

	mux.Handle("/api/", checkOrigin(api))
	mux.Handle("/", s.spa())
	return logRequests(securityHeaders(mux))
}

// spa serves the built frontend, falling back to index.html for client routes.
func (s *Server) spa() http.Handler {
	files := http.FileServerFS(s.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Web, p); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			p = "index.html"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// checkOrigin rejects cross-site state-changing requests. The progress beacon
// arrives as a text/plain "simple" request that browsers send cross-site
// without a preflight, so the Origin (or Sec-Fetch-Site) check is the CSRF guard.
func checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeError(w, http.StatusForbidden, "cross-site request refused")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			host := r.Header.Get("X-Forwarded-Host")
			if host == "" {
				host = r.Host
			}
			if err != nil || !strings.EqualFold(u.Host, host) {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int)        { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/api/healthz" {
			return
		}
		level := slog.LevelDebug
		if sw.status >= 500 {
			level = slog.LevelError
		} else if strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.URL.Path, "/pages/") && !strings.HasSuffix(r.URL.Path, "/cover") {
			level = slog.LevelInfo
		}
		slog.Log(r.Context(), level, "http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "took", time.Since(start).Round(time.Microsecond))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs err and hides details from the client.
func serverError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// readJSON decodes a body sent as application/json or text/plain (sendBeacon).
func readJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}
