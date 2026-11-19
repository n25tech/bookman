// Package library scans the configured folders, imports books and watches for changes.
package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/n25tech/bookman/internal/config"
	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
)

// CoverQueue receives books that need a thumbnail.
type CoverQueue interface{ Enqueue(bookID int64) }

// Scanner imports books from the library folders into the database.
type Scanner struct {
	db     *db.DB
	libs   []config.Library
	ids    map[string]int64 // library root -> id
	covers CoverQueue
	mu     sync.Mutex // one scan at a time
	status ScanStatus
	stMu   sync.Mutex
}

// ScanStatus is shown to admins.
type ScanStatus struct {
	Running    bool   `json:"running"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Seen       int    `json:"seen"`
	Added      int    `json:"added"`
	Moved      int    `json:"moved"`
	Missing    int    `json:"missing"`
	Errors     int    `json:"errors"`
	LastError  string `json:"last_error,omitempty"`
}

func NewScanner(d *db.DB, libs []config.Library, covers CoverQueue) *Scanner {
	return &Scanner{db: d, libs: libs, ids: map[string]int64{}, covers: covers}
}

// Init records the configured libraries in the database.
func (s *Scanner) Init(ctx context.Context) error {
	for _, l := range s.libs {
		id, err := s.db.UpsertLibrary(ctx, l.Name, l.Path)
		if err != nil {
			return err
		}
		s.ids[l.Path] = id
	}
	return nil
}

// Roots returns the configured library roots.
func (s *Scanner) Roots() []string {
	out := make([]string, len(s.libs))
	for i, l := range s.libs {
		out[i] = l.Path
	}
	return out
}

// LibraryIDs returns the database ids of the configured libraries.
func (s *Scanner) LibraryIDs() []int64 {
	out := make([]int64, 0, len(s.ids))
	for _, id := range s.ids {
		out = append(out, id)
	}
	return out
}

func (s *Scanner) Status() ScanStatus {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	return s.status
}

func (s *Scanner) bump(f func(*ScanStatus)) {
	s.stMu.Lock()
	f(&s.status)
	s.stMu.Unlock()
}

// libraryFor returns the library a path belongs to: the deepest root that
// contains it, unless the path is inside one of that library's excludes.
func (s *Scanner) libraryFor(path string) (config.Library, bool) {
	var best config.Library
	found := false
	for _, l := range s.libs {
		rel, err := filepath.Rel(l.Path, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		if excluded(l, rel) {
			continue
		}
		if !found || len(l.Path) > len(best.Path) {
			best, found = l, true
		}
	}
	return best, found
}

func excluded(l config.Library, rel string) bool {
	for _, e := range l.Exclude {
		if rel == e || strings.HasPrefix(rel, e+"/") {
			return true
		}
	}
	return false
}

func hidden(name string) bool { return strings.HasPrefix(name, ".") }

// ScanAll scans every library. It returns when the scan is done.
func (s *Scanner) ScanAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bump(func(st *ScanStatus) { *st = ScanStatus{Running: true, StartedAt: db.Now()} })
	start := time.Now()
	for _, l := range s.libs {
		if err := s.scanLibrary(ctx, l); err != nil {
			slog.Error("scan failed", "library", l.Name, "err", err)
			s.bump(func(st *ScanStatus) { st.Errors++; st.LastError = err.Error() })
		}
	}
	s.bump(func(st *ScanStatus) { st.Running = false; st.FinishedAt = db.Now() })
	st := s.Status()
	slog.Info("scan finished", "took", time.Since(start).Round(time.Millisecond), "seen", st.Seen,
		"added", st.Added, "moved", st.Moved, "missing", st.Missing, "errors", st.Errors)
}

func (s *Scanner) scanLibrary(ctx context.Context, l config.Library) error {
	if _, err := os.Stat(l.Path); err != nil {
		return fmt.Errorf("library root: %w", err)
	}
	known, err := s.db.BooksUnder(ctx, l.Path)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	seenIDs := map[int64]bool{} // books found in this walk, under whatever path
	err = filepath.WalkDir(l.Path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			slog.Warn("walk", "path", path, "err", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path != l.Path && hidden(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(l.Path, path)
		if d.IsDir() {
			if path != l.Path && excluded(l, rel) {
				return fs.SkipDir
			}
			return nil
		}
		if formats.FormatOf(path) == "" || !d.Type().IsRegular() {
			return nil
		}
		// Another, deeper library owns this path.
		if owner, ok := s.libraryFor(path); !ok || owner.Path != l.Path {
			return nil
		}
		seen[path] = true
		s.bump(func(st *ScanStatus) { st.Seen++ })
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if k, ok := known[path]; ok && k.Size == info.Size() && k.Mtime == info.ModTime().UnixMilli() && k.Status != "missing" {
			seenIDs[k.ID] = true
			return nil // unchanged
		}
		id, err := s.importFile(ctx, l, path)
		if id != 0 {
			seenIDs[id] = true
		}
		if err != nil {
			slog.Warn("import failed", "path", path, "err", err)
			s.bump(func(st *ScanStatus) { st.Errors++; st.LastError = err.Error() })
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Books that were under this root but are gone (or now excluded).
	var gone []int64
	for path, k := range known {
		if seen[path] || seenIDs[k.ID] || k.Status == "missing" {
			continue
		}
		if owner, ok := s.libraryFor(path); ok && owner.Path != l.Path {
			continue // belongs to a nested library, which scans it
		}
		gone = append(gone, k.ID)
	}
	if len(gone) > 0 {
		s.bump(func(st *ScanStatus) { st.Missing += len(gone) })
	}
	return s.db.MarkMissing(ctx, gone)
}

// ImportPath imports or re-checks one file (used by the watcher).
func (s *Scanner) ImportPath(ctx context.Context, path string) error {
	l, ok := s.libraryFor(path)
	if !ok || formats.FormatOf(path) == "" || hidden(filepath.Base(path)) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.importFile(ctx, l, path)
	return err
}

// RemovedPath marks books at path, or anywhere below it, as missing.
func (s *Scanner) RemovedPath(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	if b, err := s.db.BookByPath(ctx, path); err == nil {
		ids = append(ids, b.ID)
	}
	under, err := s.db.BooksUnder(ctx, path)
	if err != nil {
		return err
	}
	for p, k := range under {
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			ids = append(ids, k.ID)
		}
	}
	return s.db.MarkMissing(ctx, ids)
}

func (s *Scanner) seriesFor(ctx context.Context, l config.Library, path string) (int64, error) {
	dir := filepath.Dir(path)
	rel, err := filepath.Rel(l.Path, dir)
	if err != nil {
		return 0, err
	}
	name := filepath.Base(dir)
	if rel == "." {
		name = l.Name
	}
	return s.db.UpsertSeries(ctx, s.ids[l.Path], name, rel)
}

// importFile adds or updates the book at path and returns its id (0 if it was skipped).
func (s *Scanner) importFile(ctx context.Context, l config.Library, path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	hash, size, err := ContentHash(path)
	if err != nil {
		return 0, err
	}
	seriesID, err := s.seriesFor(ctx, l, path)
	if err != nil {
		return 0, err
	}
	libID := s.ids[l.Path]
	mtime := info.ModTime().UnixMilli()

	existing, err := s.db.BookByHash(ctx, hash)
	switch {
	case err == nil:
		if existing.Path != path {
			// Same content elsewhere: a move if the old file is gone, else a duplicate.
			if _, statErr := os.Stat(existing.Path); statErr == nil && existing.Status != "missing" {
				slog.Debug("duplicate file ignored", "path", path, "same_as", existing.Path)
				return 0, nil
			}
			s.bump(func(st *ScanStatus) { st.Moved++ })
			slog.Info("book moved", "from", existing.Path, "to", path)
		}
		return existing.ID, s.db.MoveBook(ctx, existing.ID, libID, seriesID, path, mtime)
	case !errors.Is(err, db.ErrNotFound):
		return 0, err
	}

	// A file whose content changed at the same path: retire the old row.
	if old, err := s.db.BookByPath(ctx, path); err == nil && old.ContentHash != hash {
		if err := s.db.MarkMissing(ctx, []int64{old.ID}); err != nil {
			return 0, err
		}
	}

	nb := db.NewBook{
		LibraryID: libID, SeriesID: seriesID, ContentHash: hash, Path: path,
		Format: formats.FormatOf(path), Title: formats.TitleFromFilename(path),
		FileSize: size, Mtime: mtime, Status: "ok",
	}
	if err := readBook(path, &nb); err != nil {
		nb.Status, nb.ErrorMsg = "error", err.Error()
		s.bump(func(st *ScanStatus) { st.Errors++; st.LastError = path + ": " + err.Error() })
	}
	id, err := s.db.InsertBook(ctx, nb)
	if err != nil {
		return 0, err
	}
	s.bump(func(st *ScanStatus) { st.Added++ })
	if nb.Status == "ok" && s.covers != nil {
		s.covers.Enqueue(id)
	}
	return id, nil
}

// readBook fills page count, metadata and comic pages from the file.
func readBook(path string, nb *db.NewBook) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reading book: %v", r)
		}
	}()
	src, err := formats.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	n, err := src.PageCount()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no pages")
	}
	nb.PageCount = n
	if md, err := src.Metadata(); err == nil {
		if t := strings.TrimSpace(md.Title); t != "" && !junkTitle(t) {
			nb.Title = t
		}
		nb.Author = strings.TrimSpace(md.Author)
	}
	if c, ok := src.(formats.Comic); ok {
		nb.Pages = c.Pages()
	}
	return nil
}

// junkTitle catches PDF titles that are clearly not the book's title.
func junkTitle(t string) bool {
	l := strings.ToLower(t)
	return l == "untitled" || strings.HasPrefix(l, "microsoft word") || strings.HasSuffix(l, ".pdf") ||
		strings.HasSuffix(l, ".doc") || strings.HasSuffix(l, ".indd") || len(t) < 2
}
