package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/n25tech/bookman/internal/auth"
	"github.com/n25tech/bookman/internal/db"
)

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		hash := auth.HashToken(c.Value)
		u, exp, err := s.DB.SessionUser(r.Context(), hash)
		if err != nil {
			if err == db.ErrNotFound {
				writeError(w, http.StatusUnauthorized, "session expired")
				return
			}
			serverError(w, r, err)
			return
		}
		// Sliding expiry: refresh when less than half the lifetime is left.
		if time.Until(time.UnixMilli(exp)) < sessionTTL/2 {
			newExp := time.Now().Add(sessionTTL)
			if err := s.DB.ExtendSessionExpiry(r.Context(), hash, newExp.UnixMilli()); err == nil {
				setSessionCookie(w, r, c.Value, newExp)
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r).IsAdmin {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: exp, MaxAge: int(time.Until(exp).Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isTLS(r),
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u db.User) {
	token, hash, err := auth.NewToken()
	if err != nil {
		serverError(w, r, err)
		return
	}
	exp := time.Now().Add(sessionTTL)
	if err := s.DB.CreateSession(r.Context(), hash, u.ID, exp.UnixMilli()); err != nil {
		serverError(w, r, err)
		return
	}
	setSessionCookie(w, r, token, exp)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.DB.UserCount(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": n == 0})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

func (c *credentials) clean() string {
	c.Username = strings.TrimSpace(c.Username)
	if c.Username == "" || len(c.Username) > 64 {
		return "username is required (max 64 characters)"
	}
	if err := auth.ValidatePassword(c.Password); err != nil {
		return err.Error()
	}
	return ""
}

// setup creates the first admin; it only works while there are no users.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !readJSON(w, r, maxJSONBody, &c) {
		return
	}
	if msg := c.clean(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		serverError(w, r, err)
		return
	}
	u, created, err := s.DB.CreateFirstAdmin(r.Context(), c.Username, hash)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !created {
		writeError(w, http.StatusConflict, "setup already done")
		return
	}
	s.startSession(w, r, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !readJSON(w, r, maxJSONBody, &c) {
		return
	}
	u, err := s.DB.UserByName(r.Context(), strings.TrimSpace(c.Username))
	if err != nil && err != db.ErrNotFound {
		serverError(w, r, err)
		return
	}
	if err == db.ErrNotFound {
		// Spend the same time as a real check so usernames can't be probed by timing.
		auth.CheckPassword("$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g", c.Password)
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, c.Password) {
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	s.startSession(w, r, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.DB.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isTLS(r)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, userFrom(r)) }

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.DB.Users(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !readJSON(w, r, maxJSONBody, &c) {
		return
	}
	if msg := c.clean(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := s.DB.UserByName(r.Context(), c.Username); err == nil {
		writeError(w, http.StatusConflict, "username taken")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		serverError(w, r, err)
		return
	}
	u, err := s.DB.CreateUser(r.Context(), c.Username, hash, c.IsAdmin)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) adminScan(w http.ResponseWriter, r *http.Request) {
	if s.Scanner.Status().Running {
		writeJSON(w, http.StatusAccepted, s.scanStatus())
		return
	}
	go s.ScanAll()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

func (s *Server) scanStatus() map[string]any {
	return map[string]any{"scan": s.Scanner.Status(), "covers_pending": s.Thumbs.Pending()}
}

func (s *Server) adminScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.scanStatus())
}
