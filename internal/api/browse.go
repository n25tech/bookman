package api

import (
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
)

// Folder is a sub-folder in the library browser. Counts include everything below it.
type Folder struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"` // relative to the library root, "/"-separated
	BookCount     int     `json:"book_count"`
	FinishedCount int     `json:"finished_count"`
	ReadingCount  int     `json:"reading_count"`
	CoverBookID   int64   `json:"cover_book_id"`
	CoverETag     *string `json:"cover_etag"`
	coverRel      string
}

// browse lists one folder of a library: its sub-folders and the books directly in it,
// mirroring the layout on disk.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	lib, err := s.DB.Library(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	dir := strings.Trim(path.Clean("/"+r.URL.Query().Get("path")), "/") // "" is the root; no ".." escapes
	books, err := s.DB.LibraryBooks(r.Context(), userFrom(r).ID, id, "", "")
	if err != nil {
		serverError(w, r, err)
		return
	}

	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	folders := map[string]*Folder{}
	direct := []db.Book{}
	for i := range books {
		b := &books[i]
		s.relPath(lib, b)
		rel := strings.ReplaceAll(b.RelPath, "\\", "/")
		if !strings.HasPrefix(rel, prefix) {
			continue
		}
		rest := rel[len(prefix):]
		name, _, nested := strings.Cut(rest, "/")
		if !nested {
			direct = append(direct, *b)
			continue
		}
		f := folders[name]
		if f == nil {
			f = &Folder{Name: name, Path: prefix + name}
			folders[name] = f
		}
		f.BookCount++
		switch bookState(*b) {
		case "finished":
			f.FinishedCount++
		case "reading":
			f.ReadingCount++
		}
		// The cover is the first book in natural order that has one.
		if b.CoverETag != nil && (f.CoverETag == nil || formats.NaturalLess(rel, f.coverRel)) {
			f.CoverBookID, f.CoverETag, f.coverRel = b.ID, b.CoverETag, rel
		}
	}
	if len(direct) == 0 && len(folders) == 0 && dir != "" {
		writeError(w, http.StatusNotFound, "no such folder")
		return
	}
	list := make([]*Folder, 0, len(folders))
	for _, f := range folders {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool { return formats.NaturalLess(list[i].Name, list[j].Name) })
	sortBooks(direct, "path")

	crumbs := []map[string]string{}
	if dir != "" {
		parts := strings.Split(dir, "/")
		for i := range parts {
			crumbs = append(crumbs, map[string]string{"name": parts[i], "path": strings.Join(parts[:i+1], "/")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"library": lib, "path": dir, "breadcrumbs": crumbs, "folders": list, "books": direct,
	})
}
