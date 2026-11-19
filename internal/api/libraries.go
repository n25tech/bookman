package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
)

// adminLibrary is a library as the admin UI sees it, with its folder.
type adminLibrary struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Exclude   []string `json:"exclude"`
	BookCount int      `json:"book_count"`
	Exists    bool     `json:"exists"` // the folder is reachable
}

type libraryInput struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Exclude []string `json:"exclude"`
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// inMediaRoots reports whether p (and where its symlinks lead) is inside a media root.
func (s *Server) inMediaRoots(p string) bool {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		real = p
	}
	for _, r := range s.MediaRoots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			rr = r
		}
		if within(r, p) && within(rr, real) {
			return true
		}
	}
	return false
}

// validate checks a library form and returns the cleaned values, or a message for the user.
func (s *Server) validate(in libraryInput) (libraryInput, string) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return in, "a name is required (max 100 characters)"
	}
	if !filepath.IsAbs(strings.TrimSpace(in.Path)) {
		return in, "choose a folder"
	}
	in.Path = filepath.Clean(strings.TrimSpace(in.Path))
	if !s.inMediaRoots(in.Path) {
		return in, "that folder is outside the folders bookman may use (media_roots: " + strings.Join(s.MediaRoots, ", ") + ")"
	}
	if st, err := os.Stat(in.Path); err != nil || !st.IsDir() {
		return in, "that folder doesn't exist or can't be read"
	}
	seen := map[string]bool{}
	excl := []string{}
	for _, e := range in.Exclude {
		e = strings.Trim(strings.TrimSpace(e), "/")
		if e == "" {
			continue
		}
		e = filepath.Clean(e)
		if filepath.IsAbs(e) || e == ".." || strings.HasPrefix(e, "../") {
			return in, "skipped folders must be inside the library folder: " + e
		}
		if !seen[e] {
			seen[e] = true
			excl = append(excl, e)
		}
	}
	in.Exclude = excl
	return in, ""
}

func (s *Server) adminLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.DB.Libraries(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	out := make([]adminLibrary, 0, len(libs))
	for _, l := range libs {
		st, err := os.Stat(l.RootPath)
		out = append(out, adminLibrary{ID: l.ID, Name: l.Name, Path: l.RootPath, Exclude: nonNil(l.Exclude),
			BookCount: l.BookCount, Exists: err == nil && st.IsDir()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": out, "media_roots": s.MediaRoots})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// afterChange reloads the scanner's libraries (restarting the watcher) and scans id.
func (s *Server) afterChange(r *http.Request, id int64) bool {
	if err := s.Scanner.Reload(r.Context()); err != nil {
		return false
	}
	if id != 0 && s.ScanLibrary != nil {
		go s.ScanLibrary(id)
	}
	return true
}

func (s *Server) adminCreateLibrary(w http.ResponseWriter, r *http.Request) {
	var in libraryInput
	if !readJSON(w, r, maxJSONBody, &in) {
		return
	}
	in, msg := s.validate(in)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	id, err := s.DB.AddLibrary(r.Context(), in.Name, in.Path, in.Exclude)
	if errors.Is(err, db.ErrConflict) {
		writeError(w, http.StatusConflict, "another library already uses that folder")
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	s.afterChange(r, id)
	writeJSON(w, http.StatusCreated, adminLibrary{ID: id, Name: in.Name, Path: in.Path, Exclude: in.Exclude, Exists: true})
}

func (s *Server) adminUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in libraryInput
	if !readJSON(w, r, maxJSONBody, &in) {
		return
	}
	in, msg := s.validate(in)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	err := s.DB.UpdateLibrary(r.Context(), id, in.Name, in.Path, in.Exclude)
	if errors.Is(err, db.ErrConflict) {
		writeError(w, http.StatusConflict, "another library (or a removed one) already uses that folder")
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	s.afterChange(r, id)
	writeJSON(w, http.StatusOK, adminLibrary{ID: id, Name: in.Name, Path: in.Path, Exclude: in.Exclude, Exists: true})
}

func (s *Server) adminRemoveLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.DB.RemoveLibrary(r.Context(), id); err != nil {
		serverError(w, r, err)
		return
	}
	s.afterChange(r, 0)
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) adminScanLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if s.Scanner.Status().Running {
		writeError(w, http.StatusConflict, "a scan is already running")
		return
	}
	found := false
	for _, l := range s.Scanner.Libraries() {
		found = found || l.ID == id
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such library")
		return
	}
	go s.ScanLibrary(id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

type fsEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Library string `json:"library,omitempty"` // name of the library using this folder
}

// adminFS lists sub-folders for the folder picker, limited to the media roots.
func (s *Server) adminFS(w http.ResponseWriter, r *http.Request) {
	libByPath := map[string]string{}
	for _, l := range s.Scanner.Libraries() {
		libByPath[l.RootPath] = l.Name
	}
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		dirs := []fsEntry{}
		for _, root := range s.MediaRoots {
			dirs = append(dirs, fsEntry{Name: root, Path: root, Library: libByPath[root]})
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": "", "parent": nil, "dirs": dirs})
		return
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || !s.inMediaRoots(p) {
		writeError(w, http.StatusForbidden, "outside the media roots")
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "can't read that folder")
		return
	}
	dirs := []fsEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(p, e.Name())
		if !e.IsDir() {
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			if st, err := os.Stat(full); err != nil || !st.IsDir() {
				continue
			}
		}
		dirs = append(dirs, fsEntry{Name: e.Name(), Path: full, Library: libByPath[full]})
	}
	sort.Slice(dirs, func(i, j int) bool { return formats.NaturalLess(dirs[i].Name, dirs[j].Name) })
	var parent *string
	for _, root := range s.MediaRoots {
		if p == root {
			empty := ""
			parent = &empty // up goes to the list of roots
		}
	}
	if parent == nil {
		up := filepath.Dir(p)
		parent = &up
	}
	books := 0
	for _, e := range entries {
		if !e.IsDir() && formats.FormatOf(e.Name()) != "" {
			books++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs, "books_here": books, "library": libByPath[p]})
}
