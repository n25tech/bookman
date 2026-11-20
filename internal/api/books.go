package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
	"github.com/n25tech/bookman/internal/progress"
)

func (s *Server) libraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.DB.Libraries(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	if libs == nil {
		libs = []db.Library{}
	}
	writeJSON(w, http.StatusOK, libs)
}

func (s *Server) relPath(lib db.Library, b *db.Book) {
	if rel, err := filepath.Rel(lib.RootPath, b.Path); err == nil {
		b.RelPath = rel
	}
}

// bookState is unread / reading / finished for the status filter.
func bookState(b db.Book) string {
	switch {
	case b.Progress == nil:
		return "unread"
	case b.Progress.Finished:
		return "finished"
	default:
		return "reading"
	}
}

func sortBooks(books []db.Book, by string) {
	switch by {
	case "added":
		sort.SliceStable(books, func(i, j int) bool { return books[i].AddedAt > books[j].AddedAt })
	case "last_read":
		last := func(b db.Book) int64 {
			if b.Progress == nil {
				return 0
			}
			return b.Progress.UpdatedAt
		}
		sort.SliceStable(books, func(i, j int) bool { return last(books[i]) > last(books[j]) })
	case "path":
		sort.SliceStable(books, func(i, j int) bool { return formats.NaturalLess(books[i].RelPath, books[j].RelPath) })
	default:
		sort.SliceStable(books, func(i, j int) bool { return formats.NaturalLess(books[i].Title, books[j].Title) })
	}
}

func (s *Server) libraryBooks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	lib, err := s.DB.Library(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	q := r.URL.Query()
	books, err := s.DB.LibraryBooks(r.Context(), userFrom(r).ID, id, strings.TrimSpace(q.Get("q")), q.Get("format"))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if status := q.Get("status"); status != "" {
		filtered := books[:0]
		for _, b := range books {
			if bookState(b) == status {
				filtered = append(filtered, b)
			}
		}
		books = filtered
	}
	for i := range books {
		s.relPath(lib, &books[i])
	}
	sortBooks(books, q.Get("sort"))
	total := len(books)
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if offset > 0 && offset < len(books) {
		books = books[offset:]
	} else if offset >= len(books) {
		books = books[:0]
	}
	if limit > 0 && limit < len(books) {
		books = books[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"library": lib, "total": total, "books": books})
}

func (s *Server) librarySeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	series, err := s.DB.LibrarySeries(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	sort.SliceStable(series, func(i, j int) bool { return formats.NaturalLess(series[i].Name, series[j].Name) })
	if series == nil {
		series = []db.Series{}
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	se, err := s.DB.Series(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	lib, err := s.DB.Library(r.Context(), se.LibraryID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	books, err := s.DB.SeriesBooks(r.Context(), userFrom(r).ID, id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for i := range books {
		s.relPath(lib, &books[i])
	}
	sortBooks(books, "path")
	se.BookCount = len(books)
	writeJSON(w, http.StatusOK, map[string]any{"series": se, "library": lib, "books": books})
}

// loadBook fetches the book in the path for the signed-in user.
func (s *Server) loadBook(w http.ResponseWriter, r *http.Request) (db.Book, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return db.Book{}, false
	}
	b, err := s.DB.Book(r.Context(), userFrom(r).ID, id)
	if err != nil {
		serverError(w, r, err)
		return b, false
	}
	return b, true
}

func (s *Server) book(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	lib, err := s.DB.Library(r.Context(), b.LibraryID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	s.relPath(lib, &b)
	notes, err := s.DB.NoteCount(r.Context(), userFrom(r).ID, b.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"book": b, "library": lib, "note_count": notes})
}

const maxMetadataLen = 500

// updateBook edits the title and author (admin only). Only the database changes;
// the file is left alone so its content hash, and the progress tied to it, stay put.
func (s *Server) updateBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Title  *string `json:"title"`
		Author *string `json:"author"`
	}
	if !readJSON(w, r, maxJSONBody, &in) {
		return
	}
	b, err := s.DB.Book(r.Context(), userFrom(r).ID, id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	title, author := b.Title, b.Author
	if in.Title != nil {
		title = strings.Join(strings.Fields(formats.CleanText(*in.Title)), " ")
	}
	if in.Author != nil {
		author = strings.Join(strings.Fields(formats.CleanText(*in.Author)), " ")
	}
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(title) > maxMetadataLen || len(author) > maxMetadataLen {
		writeError(w, http.StatusBadRequest, "title and author must be at most 500 bytes")
		return
	}
	if err := s.DB.UpdateBookMetadata(r.Context(), id, title, author); err != nil {
		serverError(w, r, err)
		return
	}
	s.book(w, r)
}

func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	etag, img, err := s.DB.Cover(r.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			writeError(w, http.StatusNotFound, "no cover yet")
			return
		}
		serverError(w, r, err)
		return
	}
	quoted := `"` + etag + `"`
	w.Header().Set("ETag", quoted)
	if r.URL.Query().Get("v") == etag {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	if r.Header.Get("If-None-Match") == quoted {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(img)))
	w.Write(img)
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	f, err := os.Open(b.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file is missing")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		serverError(w, r, err)
		return
	}
	switch b.Format {
	case "pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case "cbz":
		w.Header().Set("Content-Type", "application/vnd.comicbook+zip")
	case "cbr":
		w.Header().Set("Content-Type", "application/vnd.comicbook-rar")
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(b.Path)))
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// page serves comic page n (1-based) from the entry recorded at import.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	if b.Format != "cbz" && b.Format != "cbr" {
		writeError(w, http.StatusBadRequest, "pages are only served for comics")
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 || n > b.PageCount {
		writeError(w, http.StatusNotFound, "no such page")
		return
	}
	key := func(entry string) string { return b.ContentHash + "\x00" + entry }
	stored, err := s.DB.ComicPageEntry(r.Context(), b.ID, n-1)
	if err != nil {
		serverError(w, r, err)
		return
	}
	entry := formats.DecodeEntryName(stored)
	data, ct, hit := s.Pages.Get(key(entry))
	if !hit {
		if b.Format == "cbr" {
			data, ct, err = formats.ReadRarEntryCaching(b.Path, entry, func(name string, d []byte) {
				s.Pages.Put(key(name), d, formats.ContentType(name))
			})
		} else {
			data, ct, err = formats.ReadEntry(b.Path, entry)
		}
		if err != nil {
			serverError(w, r, err)
			return
		}
		s.Pages.Put(key(entry), data, ct)
	}
	etag := fmt.Sprintf(`"%s-%d"`, b.ContentHash[:16], n)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func (s *Server) getProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, b.Progress) // null when never opened
}

func (s *Server) putProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	var in progress.Input
	if !readJSON(w, r, progressBody, &in) {
		return
	}
	if in.ClientTS <= 0 || in.Page < 1 {
		writeError(w, http.StatusBadRequest, "page and client_ts are required")
		return
	}
	// A client clock far in the future would lock the row; refuse it.
	if in.ClientTS > time.Now().Add(24*time.Hour).UnixMilli() {
		writeError(w, http.StatusBadRequest, "client_ts is in the future")
		return
	}
	if len(in.Device) > 100 {
		in.Device = in.Device[:100]
	}
	p, err := progress.Save(r.Context(), s.DB, userFrom(r).ID, b.ID, b.PageCount, in)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	if err := s.DB.DeleteProgress(r.Context(), userFrom(r).ID, b.ID); err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (s *Server) finished(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	body := struct {
		Finished *bool `json:"finished"`
	}{}
	if r.ContentLength != 0 && !readJSON(w, r, maxJSONBody, &body) {
		return
	}
	finished := body.Finished == nil || *body.Finished
	p, err := s.DB.SetFinished(r.Context(), userFrom(r).ID, b.ID, b.PageCount, finished)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	list, err := s.DB.Sessions(r.Context(), userFrom(r).ID, b.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) continueReading(w http.ResponseWriter, r *http.Request) {
	books, err := s.DB.ContinueReading(r.Context(), userFrom(r).ID, s.Scanner.LibraryIDs(), 24)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}

func (s *Server) recentlyAdded(w http.ResponseWriter, r *http.Request) {
	books, err := s.DB.RecentlyAdded(r.Context(), userFrom(r).ID, s.Scanner.LibraryIDs(), 24)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}
