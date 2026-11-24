package db

import (
	"context"
	"database/sql"
)

type Progress struct {
	BookID    int64   `json:"book_id"`
	Page      int     `json:"page"`
	Offset    float64 `json:"offset"`
	Percent   float64 `json:"percent"`
	Finished  bool    `json:"finished"`
	ClientTS  int64   `json:"client_ts"`
	Device    string  `json:"device"`
	UpdatedAt int64   `json:"updated_at"`
}

type ReadingSession struct {
	ID        int64  `json:"id"`
	Device    string `json:"device"`
	StartPage int    `json:"start_page"`
	EndPage   int    `json:"end_page"`
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
}

type Note struct {
	ID          int64   `json:"id"`
	BookID      int64   `json:"book_id"`
	Page        int     `json:"page"`
	BodyMD      string  `json:"body_md"`
	Quote       *string `json:"quote"`
	AnchorStart *int    `json:"anchor_start"`
	AnchorEnd   *int    `json:"anchor_end"`
	Color       *string `json:"color"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
	DeletedAt   *int64  `json:"deleted_at,omitempty"`
	// ClientID is chosen by the browser so a retried create is idempotent.
	ClientID *string `json:"client_id"`
}

// Querier is satisfied by *sql.DB, *sql.Tx and *sql.Conn.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// GetProgress reads one progress row; lock=true takes a row lock (inside a tx).
func GetProgress(ctx context.Context, q Querier, userID, bookID int64, lock bool) (*Progress, error) {
	query := `SELECT book_id, page, "offset", percent, finished, client_ts, device, updated_at FROM progress WHERE user_id = $1 AND book_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var p Progress
	err := q.QueryRowContext(ctx, query, userID, bookID).Scan(&p.BookID, &p.Page, &p.Offset, &p.Percent, &p.Finished, &p.ClientTS, &p.Device, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// PutProgress upserts a row, but only over an older client_ts (last write wins).
func PutProgress(ctx context.Context, q Querier, userID int64, p Progress) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO progress (user_id, book_id, page, "offset", percent, finished, client_ts, device, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (user_id, book_id) DO UPDATE SET
			page = EXCLUDED.page, "offset" = EXCLUDED."offset", percent = EXCLUDED.percent,
			finished = EXCLUDED.finished, client_ts = EXCLUDED.client_ts, device = EXCLUDED.device,
			updated_at = EXCLUDED.updated_at
		WHERE progress.client_ts < EXCLUDED.client_ts`,
		userID, p.BookID, p.Page, p.Offset, p.Percent, p.Finished, p.ClientTS, p.Device, p.UpdatedAt)
	return err
}

// SetFinished sets the finished flag, creating a row at the last page if none exists.
// client_ts is left alone so it never blocks the reader's next save.
func (d *DB) SetFinished(ctx context.Context, userID, bookID int64, pageCount int, finished bool) (*Progress, error) {
	now := Now()
	page := max(pageCount, 1)
	_, err := d.ExecContext(ctx, `
		INSERT INTO progress (user_id, book_id, page, "offset", percent, finished, client_ts, device, updated_at)
		VALUES ($1, $2, $3, CASE WHEN $4 THEN 1 ELSE 0 END, CASE WHEN $4 THEN 1 ELSE 0 END, $4, 0, '', $5)
		ON CONFLICT (user_id, book_id) DO UPDATE SET finished = EXCLUDED.finished, updated_at = EXCLUDED.updated_at`,
		userID, bookID, page, finished, now)
	if err != nil {
		return nil, err
	}
	return GetProgress(ctx, d, userID, bookID, false)
}

func (d *DB) DeleteProgress(ctx context.Context, userID, bookID int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM progress WHERE user_id = $1 AND book_id = $2`, userID, bookID)
	return err
}

// LastSession is the device's most recent reading session for a book.
func LastSession(ctx context.Context, q Querier, userID, bookID int64, device string) (*ReadingSession, error) {
	var s ReadingSession
	err := q.QueryRowContext(ctx, `
		SELECT id, device, start_page, end_page, started_at, ended_at FROM reading_sessions
		WHERE user_id = $1 AND book_id = $2 AND device = $3 ORDER BY ended_at DESC LIMIT 1`,
		userID, bookID, device).Scan(&s.ID, &s.Device, &s.StartPage, &s.EndPage, &s.StartedAt, &s.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func ExtendSession(ctx context.Context, q Querier, id int64, page int, ts int64) error {
	_, err := q.ExecContext(ctx, `UPDATE reading_sessions SET end_page = $2, ended_at = $3 WHERE id = $1`, id, page, ts)
	return err
}

func StartSession(ctx context.Context, q Querier, userID, bookID int64, device string, page int, ts int64) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO reading_sessions (user_id, book_id, device, start_page, end_page, started_at, ended_at)
		VALUES ($1, $2, $3, $4, $4, $5, $5)`, userID, bookID, device, page, ts)
	return err
}

func (d *DB) Sessions(ctx context.Context, userID, bookID int64) ([]ReadingSession, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, device, start_page, end_page, started_at, ended_at FROM reading_sessions
		WHERE user_id = $1 AND book_id = $2 ORDER BY started_at DESC LIMIT 200`, userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReadingSession{}
	for rows.Next() {
		var s ReadingSession
		if err := rows.Scan(&s.ID, &s.Device, &s.StartPage, &s.EndPage, &s.StartedAt, &s.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const noteColumns = `id, book_id, page, body_md, quote, anchor_start, anchor_end, color, created_at, updated_at, deleted_at, client_id`

func scanNote(r scanner) (Note, error) {
	var n Note
	err := r.Scan(&n.ID, &n.BookID, &n.Page, &n.BodyMD, &n.Quote, &n.AnchorStart, &n.AnchorEnd, &n.Color, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt, &n.ClientID)
	return n, err
}

func (d *DB) Notes(ctx context.Context, userID, bookID int64) ([]Note, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+noteColumns+` FROM notes
		WHERE user_id = $1 AND book_id = $2 AND deleted_at IS NULL
		ORDER BY page, COALESCE(anchor_start, 0), created_at`, userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (d *DB) NoteCount(ctx context.Context, userID, bookID int64) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT count(*) FROM notes WHERE user_id = $1 AND book_id = $2 AND deleted_at IS NULL`, userID, bookID).Scan(&n)
	return n, err
}

// CreateNote inserts a note. A note with the same client_id already exists when
// a create is retried: that note is returned unchanged (created=false).
func (d *DB) CreateNote(ctx context.Context, userID int64, n Note) (note Note, created bool, err error) {
	now := Now()
	note, err = scanNote(d.QueryRowContext(ctx, `
		INSERT INTO notes (user_id, book_id, page, body_md, quote, anchor_start, anchor_end, color, created_at, updated_at, client_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10)
		ON CONFLICT (user_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
		RETURNING `+noteColumns,
		userID, n.BookID, n.Page, n.BodyMD, n.Quote, n.AnchorStart, n.AnchorEnd, n.Color, now, n.ClientID))
	if err == sql.ErrNoRows && n.ClientID != nil {
		note, err = scanNote(d.QueryRowContext(ctx, `SELECT `+noteColumns+` FROM notes WHERE user_id = $1 AND client_id = $2`, userID, *n.ClientID))
		return note, false, err
	}
	return note, true, err
}

// NotePatch holds the optional fields of PATCH /notes/{id}.
type NotePatch struct {
	BodyMD  *string `json:"body_md"`
	Color   *string `json:"color"`
	Page    *int    `json:"page"`
	Deleted *bool   `json:"deleted"` // false restores a soft-deleted note (undo)
}

func (d *DB) UpdateNote(ctx context.Context, userID, id int64, p NotePatch) (Note, error) {
	n, err := scanNote(d.QueryRowContext(ctx, `
		UPDATE notes SET
			body_md = COALESCE($3, body_md),
			color = CASE WHEN $4::text IS NULL THEN color WHEN $4 = '' THEN NULL ELSE $4 END,
			page = COALESCE($5, page),
			deleted_at = CASE WHEN $6::boolean IS NULL THEN deleted_at WHEN $6 THEN $7 ELSE NULL END,
			updated_at = $7
		WHERE id = $1 AND user_id = $2 RETURNING `+noteColumns,
		id, userID, p.BodyMD, p.Color, p.Page, p.Deleted, Now()))
	return n, notFound(err)
}

// Users and sessions.

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	IsAdmin      bool   `json:"is_admin"`
	CreatedAt    int64  `json:"created_at"`
}

func (d *DB) UserCount(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (d *DB) CreateUser(ctx context.Context, username, hash string, admin bool) (User, error) {
	u := User{Username: username, PasswordHash: hash, IsAdmin: admin, CreatedAt: Now()}
	err := d.QueryRowContext(ctx, `INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ($1,$2,$3,$4) RETURNING id`,
		username, hash, admin, u.CreatedAt).Scan(&u.ID)
	return u, err
}

// CreateFirstAdmin creates an admin only while the users table is empty.
func (d *DB) CreateFirstAdmin(ctx context.Context, username, hash string) (User, bool, error) {
	u := User{Username: username, PasswordHash: hash, IsAdmin: true, CreatedAt: Now()}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return u, false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE users IN EXCLUSIVE MODE`); err != nil {
		return u, false, err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return u, false, err
	}
	if n > 0 {
		return u, false, nil
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO users (username, password_hash, is_admin, created_at) VALUES ($1,$2,TRUE,$3) RETURNING id`,
		username, hash, u.CreatedAt).Scan(&u.ID)
	if err != nil {
		return u, false, err
	}
	return u, true, tx.Commit()
}

func (d *DB) UserByName(ctx context.Context, username string) (User, error) {
	var u User
	err := d.QueryRowContext(ctx, `SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	return u, notFound(err)
}

func (d *DB) Users(ctx context.Context) ([]User, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, username, password_hash, is_admin, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (d *DB) CreateSession(ctx context.Context, tokenHash string, userID, expires int64) error {
	_, err := d.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES ($1,$2,$3,$4)`, tokenHash, userID, expires, Now())
	return err
}

// SessionUser returns the user of a live session and the session's expiry.
func (d *DB) SessionUser(ctx context.Context, tokenHash string) (User, int64, error) {
	var u User
	var exp int64
	err := d.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.is_admin, u.created_at, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, Now()).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &exp)
	return u, exp, notFound(err)
}

func (d *DB) ExtendSessionExpiry(ctx context.Context, tokenHash string, expires int64) error {
	_, err := d.ExecContext(ctx, `UPDATE sessions SET expires_at = $2 WHERE token_hash = $1`, tokenHash, expires)
	return err
}

func (d *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (d *DB) DeleteExpiredSessions(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, Now())
	return err
}
