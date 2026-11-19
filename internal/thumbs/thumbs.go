// Package thumbs renders cover thumbnails in a bounded worker pool and stores them.
package thumbs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/jpeg"
	"log/slog"
	"sync"

	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
)

const (
	MaxW = 400
	MaxH = 600
)

// Worker generates covers for queued books.
type Worker struct {
	db      *db.DB
	queue   chan int64
	mu      sync.Mutex
	queued  map[int64]bool
	workers int
}

func New(d *db.DB, workers int) *Worker {
	return &Worker{db: d, queue: make(chan int64, 100000), queued: map[int64]bool{}, workers: max(1, workers)}
}

// Enqueue asks for a cover; duplicates are ignored.
func (w *Worker) Enqueue(id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.queued[id] {
		return
	}
	select {
	case w.queue <- id:
		w.queued[id] = true
	default:
		slog.Warn("cover queue full", "book", id)
	}
}

// Pending is the number of covers waiting or in progress.
func (w *Worker) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.queued)
}

// Run starts the workers and queues books that have no cover yet.
func (w *Worker) Run(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-w.queue:
					if err := w.make(ctx, id); err != nil {
						slog.Warn("cover failed", "book", id, "err", err)
					}
					w.mu.Lock()
					delete(w.queued, id)
					w.mu.Unlock()
				}
			}
		}()
	}
	w.EnqueueMissing(ctx)
}

// EnqueueMissing queues every present book without a cover.
func (w *Worker) EnqueueMissing(ctx context.Context) {
	ids, err := w.db.BooksWithoutCover(ctx)
	if err != nil {
		slog.Error("listing books without cover", "err", err)
		return
	}
	for _, id := range ids {
		w.Enqueue(id)
	}
}

func (w *Worker) make(ctx context.Context, id int64) (err error) {
	b, err := w.db.Book(ctx, 0, id)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	src, err := formats.Open(b.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	img, err := src.Cover(MaxW, MaxH)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return err
	}
	sum := sha256.Sum256(buf.Bytes())
	return w.db.SaveCover(ctx, id, hex.EncodeToString(sum[:8]), buf.Bytes())
}
