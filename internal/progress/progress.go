// Package progress implements the reading-progress rules: last write wins by
// client time, auto-finish on the last page, and per-device reading sessions.
package progress

import (
	"context"
	"math"

	"github.com/n25tech/bookman/internal/db"
)

// SessionGap is how long a device may be quiet before its next save opens a new session.
const SessionGap int64 = 10 * 60 * 1000

// Input is a save from the client.
type Input struct {
	Page     int      `json:"page"`
	Offset   float64  `json:"offset"`
	ClientTS int64    `json:"client_ts"`
	Device   string   `json:"device"`
	Finished *bool    `json:"finished,omitempty"`
}

// Compute returns the row a save would produce and whether it wins over stored.
func Compute(stored *db.Progress, in Input, bookID int64, pageCount int, now int64) (db.Progress, bool) {
	if stored != nil && in.ClientTS <= stored.ClientTS {
		return *stored, false
	}
	pc := max(pageCount, 1)
	page := min(max(in.Page, 1), pc)
	offset := in.Offset
	if math.IsNaN(offset) || offset < 0 {
		offset = 0
	}
	if offset > 1 {
		offset = 1
	}
	p := db.Progress{
		BookID:    bookID,
		Page:      page,
		Offset:    offset,
		Percent:   math.Min(1, (float64(page-1)+offset)/float64(pc)),
		ClientTS:  in.ClientTS,
		Device:    in.Device,
		UpdatedAt: now,
	}
	switch {
	case in.Finished != nil:
		p.Finished = *in.Finished
	case pageCount > 0 && page == pageCount && offset >= 0.9:
		p.Finished = true
	case stored != nil:
		p.Finished = stored.Finished
	}
	return p, true
}

// SessionAction says what a save at ts does to the device's last session.
type SessionAction int

const (
	SessionNone   SessionAction = iota // older than the session's end: ignore
	SessionExtend                      // within SessionGap of the last save
	SessionNew                         // first save, or after a quiet period
)

func NextSession(last *db.ReadingSession, ts int64) SessionAction {
	switch {
	case last == nil:
		return SessionNew
	case ts < last.EndedAt:
		return SessionNone
	case ts-last.EndedAt > SessionGap:
		return SessionNew
	default:
		return SessionExtend
	}
}

// Save applies a client save in one transaction and returns the stored row.
func Save(ctx context.Context, d *db.DB, userID, bookID int64, pageCount int, in Input) (db.Progress, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return db.Progress{}, err
	}
	defer tx.Rollback()

	stored, err := db.GetProgress(ctx, tx, userID, bookID, true)
	if err != nil {
		return db.Progress{}, err
	}
	row, accepted := Compute(stored, in, bookID, pageCount, db.Now())
	if accepted {
		if err := db.PutProgress(ctx, tx, userID, row); err != nil {
			return db.Progress{}, err
		}
	}

	// Sessions use the client's clock, so a queued offline save lands at the
	// time it was made, not when it finally arrived.
	last, err := db.LastSession(ctx, tx, userID, bookID, in.Device)
	if err != nil {
		return db.Progress{}, err
	}
	page := min(max(in.Page, 1), max(pageCount, 1))
	switch NextSession(last, in.ClientTS) {
	case SessionNew:
		err = db.StartSession(ctx, tx, userID, bookID, in.Device, page, in.ClientTS)
	case SessionExtend:
		err = db.ExtendSession(ctx, tx, last.ID, page, in.ClientTS)
	}
	if err != nil {
		return db.Progress{}, err
	}

	// Read back: a concurrent newer save may have won the guarded upsert.
	final, err := db.GetProgress(ctx, tx, userID, bookID, false)
	if err != nil {
		return db.Progress{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.Progress{}, err
	}
	if final == nil {
		return row, nil
	}
	return *final, nil
}
