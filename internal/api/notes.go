package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/n25tech/bookman/internal/db"
)

const maxNoteBody = 100_000

func (s *Server) notes(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	list, err := s.DB.Notes(r.Context(), userFrom(r).ID, b.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	var n db.Note
	if !readJSON(w, r, maxJSONBody, &n) {
		return
	}
	if n.Page < 1 || n.Page > max(b.PageCount, 1) {
		writeError(w, http.StatusBadRequest, "page out of range")
		return
	}
	if len(n.BodyMD) > maxNoteBody {
		writeError(w, http.StatusBadRequest, "note too long")
		return
	}
	if n.Quote != nil && b.Format != "pdf" {
		writeError(w, http.StatusBadRequest, "text anchors are only supported for PDFs")
		return
	}
	if (n.AnchorStart == nil) != (n.AnchorEnd == nil) || (n.AnchorStart != nil && *n.AnchorStart > *n.AnchorEnd) {
		writeError(w, http.StatusBadRequest, "invalid anchor")
		return
	}
	n.BookID = b.ID
	created, err := s.DB.CreateNote(r.Context(), userFrom(r).ID, n)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var p db.NotePatch
	if !readJSON(w, r, maxJSONBody, &p) {
		return
	}
	if p.BodyMD != nil && len(*p.BodyMD) > maxNoteBody {
		writeError(w, http.StatusBadRequest, "note too long")
		return
	}
	n, err := s.DB.UpdateNote(r.Context(), userFrom(r).ID, id, p)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	del := true
	n, err := s.DB.UpdateNote(r.Context(), userFrom(r).ID, id, db.NotePatch{Deleted: &del})
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) exportNotes(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBook(w, r)
	if !ok {
		return
	}
	list, err := s.DB.Notes(r.Context(), userFrom(r).ID, b.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	md := NotesMarkdown(b, list)
	name := strings.TrimSuffix(filepath.Base(b.Path), filepath.Ext(b.Path)) + " - notes.md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Write([]byte(md))
}

// NotesMarkdown renders a book's notes as one Markdown document, grouped by page.
func NotesMarkdown(b db.Book, notes []db.Note) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", b.Title)
	if b.Author != "" {
		fmt.Fprintf(&sb, "*%s*\n\n", b.Author)
	}
	fmt.Fprintf(&sb, "%d notes, exported %s\n", len(notes), time.Now().Format("2006-01-02"))
	page := -1
	for _, n := range notes {
		if n.Page != page {
			page = n.Page
			fmt.Fprintf(&sb, "\n## Page %d\n", page)
		}
		sb.WriteString("\n")
		if n.Quote != nil && *n.Quote != "" {
			for _, line := range strings.Split(strings.TrimSpace(*n.Quote), "\n") {
				fmt.Fprintf(&sb, "> %s\n", line)
			}
			sb.WriteString("\n")
		}
		if body := strings.TrimSpace(n.BodyMD); body != "" {
			sb.WriteString(body + "\n")
		}
	}
	return sb.String()
}
