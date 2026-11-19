package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch follows changes under the library roots until ctx is done. Events are
// debounced per path, so a file being copied is imported once it settles.
func (s *Scanner) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	for _, root := range s.Roots() {
		addTree(w, root)
	}

	var mu sync.Mutex
	pending := map[string]time.Time{}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	const settle = 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			slog.Warn("watcher", "err", err)
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if hidden(filepath.Base(ev.Name)) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					addTree(w, ev.Name)
				}
			}
			mu.Lock()
			pending[ev.Name] = time.Now()
			mu.Unlock()
		case <-tick.C:
			mu.Lock()
			var ready []string
			for p, t := range pending {
				if time.Since(t) >= settle {
					ready = append(ready, p)
					delete(pending, p)
				}
			}
			mu.Unlock()
			for _, p := range ready {
				s.handlePath(ctx, p)
			}
		}
	}
}

func (s *Scanner) handlePath(ctx context.Context, p string) {
	st, err := os.Stat(p)
	switch {
	case err != nil:
		if err := s.RemovedPath(ctx, p); err != nil {
			slog.Warn("marking missing", "path", p, "err", err)
		}
	case st.IsDir():
		// A folder appeared (or was moved in): import everything below it.
		filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if err := s.ImportPath(ctx, path); err != nil {
					slog.Warn("import failed", "path", path, "err", err)
				}
			}
			return nil
		})
	default:
		if err := s.ImportPath(ctx, p); err != nil {
			slog.Warn("import failed", "path", p, "err", err)
		}
	}
}

func addTree(w *fsnotify.Watcher, root string) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root && hidden(d.Name()) {
			return fs.SkipDir
		}
		if err := w.Add(path); err != nil {
			slog.Warn("cannot watch", "path", path, "err", err)
		}
		return nil
	})
}
