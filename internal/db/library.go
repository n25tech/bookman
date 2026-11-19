package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

type Library struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	RootPath  string   `json:"-"`
	Exclude   []string `json:"-"`
	BookCount int      `json:"book_count"`
	RemovedAt *int64   `json:"-"`
}

// ErrConflict is returned when a library folder is already in use.
var ErrConflict = errors.New("conflict")

type Series struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"library_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	BookCount int    `json:"book_count"`
	// CoverBookID is the first book of the series, used for its cover.
	CoverBookID *int64  `json:"cover_book_id"`
	CoverETag   *string `json:"cover_etag"`
}

// Book is a row of books, plus the caller's progress when loaded for a user.
type Book struct {
	ID          int64     `json:"id"`
	LibraryID   int64     `json:"library_id"`
	SeriesID    *int64    `json:"series_id"`
	SeriesName  string    `json:"series_name"`
	ContentHash string    `json:"-"`
	Path        string    `json:"-"`
	RelPath     string    `json:"rel_path"`
	Format      string    `json:"format"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	PageCount   int       `json:"page_count"`
	FileSize    int64     `json:"file_size"`
	Mtime       int64     `json:"mtime"`
	Status      string    `json:"status"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
	AddedAt     int64     `json:"added_at"`
	UpdatedAt   int64     `json:"updated_at"`
	CoverETag   *string   `json:"cover_etag"`
	Progress    *Progress `json:"progress"`
}

const libraryColumns = `l.id, l.name, l.root_path, l.exclude, l.removed_at,
	(SELECT count(*) FROM books b WHERE b.library_id = l.id AND b.status <> 'missing')`

func scanLibrary(r scanner) (Library, error) {
	var l Library
	err := r.Scan(&l.ID, &l.Name, &l.RootPath, &l.Exclude, &l.RemovedAt, &l.BookCount)
	return l, err
}

// Libraries lists the libraries that have not been removed.
func (d *DB) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+libraryColumns+` FROM libraries l WHERE l.removed_at IS NULL ORDER BY l.name, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Library loads one library, removed or not.
func (d *DB) Library(ctx context.Context, id int64) (Library, error) {
	l, err := scanLibrary(d.QueryRowContext(ctx, `SELECT `+libraryColumns+` FROM libraries l WHERE l.id = $1`, id))
	return l, notFound(err)
}

func clean(exclude []string) []string {
	if exclude == nil {
		return []string{}
	}
	return exclude
}

// AddLibrary creates a library, or revives a removed one with the same folder.
// It returns ErrConflict if an active library already uses the folder.
func (d *DB) AddLibrary(ctx context.Context, name, root string, exclude []string) (int64, error) {
	var id int64
	err := d.QueryRowContext(ctx, `
		INSERT INTO libraries (name, root_path, exclude) VALUES ($1, $2, $3)
		ON CONFLICT (root_path) DO UPDATE SET name = EXCLUDED.name, exclude = EXCLUDED.exclude, removed_at = NULL
		WHERE libraries.removed_at IS NOT NULL
		RETURNING id`, name, root, clean(exclude)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrConflict
	}
	return id, err
}

// UpdateLibrary changes a library's name, folder and skipped sub-folders.
func (d *DB) UpdateLibrary(ctx context.Context, id int64, name, root string, exclude []string) error {
	res, err := d.ExecContext(ctx, `UPDATE libraries SET name = $2, root_path = $3, exclude = $4 WHERE id = $1 AND removed_at IS NULL`,
		id, name, root, clean(exclude))
	if err != nil {
		if strings.Contains(err.Error(), "23505") { // unique_violation on root_path
			return ErrConflict
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveLibrary hides a library; its books, progress and notes are kept.
func (d *DB) RemoveLibrary(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE libraries SET removed_at = $2 WHERE id = $1 AND removed_at IS NULL`, id, Now())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SeedLibrary records a library from the config file (first start only):
// an existing row with the same folder gets the configured name and excludes.
func (d *DB) SeedLibrary(ctx context.Context, name, root string, exclude []string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO libraries (name, root_path, exclude) VALUES ($1, $2, $3)
		ON CONFLICT (root_path) DO UPDATE SET name = EXCLUDED.name, exclude = EXCLUDED.exclude, removed_at = NULL`,
		name, root, clean(exclude))
	return err
}

// Setting reads a value from the settings table ("" if unset).
func (d *DB) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value)
	return err
}

// UpsertSeries returns the id of the series for a folder, creating it if needed.
func (d *DB) UpsertSeries(ctx context.Context, libraryID int64, name, path string) (int64, error) {
	var id int64
	err := d.QueryRowContext(ctx, `
		INSERT INTO series (library_id, name, path) VALUES ($1, $2, $3)
		ON CONFLICT (library_id, path) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, libraryID, name, path).Scan(&id)
	return id, err
}

func (d *DB) Series(ctx context.Context, id int64) (Series, error) {
	var s Series
	err := d.QueryRowContext(ctx, `SELECT id, library_id, name, path FROM series WHERE id = $1`, id).
		Scan(&s.ID, &s.LibraryID, &s.Name, &s.Path)
	return s, notFound(err)
}

// LibrarySeries lists the series of a library that have at least one present book.
func (d *DB) LibrarySeries(ctx context.Context, libraryID int64) ([]Series, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT s.id, s.library_id, s.name, s.path, count(b.id),
		       (array_agg(b.id ORDER BY b.path))[1]
		FROM series s JOIN books b ON b.series_id = s.id AND b.status <> 'missing'
		WHERE s.library_id = $1
		GROUP BY s.id ORDER BY s.name`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Series
	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.LibraryID, &s.Name, &s.Path, &s.BookCount, &s.CoverBookID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Attach cover etags in one query.
	ids := make([]int64, 0, len(out))
	for _, s := range out {
		if s.CoverBookID != nil {
			ids = append(ids, *s.CoverBookID)
		}
	}
	etags, err := d.coverETags(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].CoverBookID != nil {
			if e, ok := etags[*out[i].CoverBookID]; ok {
				out[i].CoverETag = &e
			}
		}
	}
	return out, nil
}

func (d *DB) coverETags(ctx context.Context, ids []int64) (map[int64]string, error) {
	m := map[int64]string{}
	if len(ids) == 0 {
		return m, nil
	}
	rows, err := d.QueryContext(ctx, `SELECT book_id, etag FROM covers WHERE book_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var e string
		if err := rows.Scan(&id, &e); err != nil {
			return nil, err
		}
		m[id] = e
	}
	return m, rows.Err()
}

const bookColumns = `b.id, b.library_id, b.series_id, COALESCE(s.name, ''), b.content_hash, b.path, b.format,
	b.title, b.author, b.page_count, b.file_size, b.mtime, b.status, b.error_msg, b.added_at, b.updated_at,
	c.etag,
	p.page, p."offset", p.percent, p.finished, p.client_ts, p.device, p.updated_at`

// bookFrom joins series, covers and the given user's progress ($1 = user id).
const bookFrom = ` FROM books b
	LEFT JOIN series s ON s.id = b.series_id
	LEFT JOIN covers c ON c.book_id = b.id
	LEFT JOIN progress p ON p.book_id = b.id AND p.user_id = $1 `

type scanner interface{ Scan(dest ...any) error }

func scanBook(r scanner) (Book, error) {
	var b Book
	var (
		page            sql.NullInt64
		offset, percent sql.NullFloat64
		finished        sql.NullBool
		clientTS, upd   sql.NullInt64
		device          sql.NullString
	)
	err := r.Scan(&b.ID, &b.LibraryID, &b.SeriesID, &b.SeriesName, &b.ContentHash, &b.Path, &b.Format,
		&b.Title, &b.Author, &b.PageCount, &b.FileSize, &b.Mtime, &b.Status, &b.ErrorMsg, &b.AddedAt, &b.UpdatedAt,
		&b.CoverETag, &page, &offset, &percent, &finished, &clientTS, &device, &upd)
	if err != nil {
		return b, err
	}
	if page.Valid {
		b.Progress = &Progress{BookID: b.ID, Page: int(page.Int64), Offset: offset.Float64, Percent: percent.Float64,
			Finished: finished.Bool, ClientTS: clientTS.Int64, Device: device.String, UpdatedAt: upd.Int64}
	}
	return b, nil
}

func (d *DB) queryBooks(ctx context.Context, query string, args ...any) ([]Book, error) {
	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Book loads one book with the user's progress.
func (d *DB) Book(ctx context.Context, userID, id int64) (Book, error) {
	b, err := scanBook(d.QueryRowContext(ctx, `SELECT `+bookColumns+bookFrom+` WHERE b.id = $2`, userID, id))
	return b, notFound(err)
}

// LibraryBooks lists present (and errored) books of a library for a user.
// Filtering by status and sorting are done by the caller.
func (d *DB) LibraryBooks(ctx context.Context, userID, libraryID int64, q, format string) ([]Book, error) {
	return d.queryBooks(ctx, `SELECT `+bookColumns+bookFrom+`
		WHERE b.library_id = $2 AND b.status <> 'missing'
		  AND ($3 = '' OR b.title ILIKE '%' || $3 || '%' OR b.author ILIKE '%' || $3 || '%')
		  AND ($4 = '' OR b.format = $4)`, userID, libraryID, q, format)
}

func (d *DB) SeriesBooks(ctx context.Context, userID, seriesID int64) ([]Book, error) {
	return d.queryBooks(ctx, `SELECT `+bookColumns+bookFrom+`
		WHERE b.series_id = $2 AND b.status <> 'missing'`, userID, seriesID)
}

// ContinueReading lists unfinished books with progress, in the given libraries.
func (d *DB) ContinueReading(ctx context.Context, userID int64, libraryIDs []int64, limit int) ([]Book, error) {
	return d.queryBooks(ctx, `SELECT `+bookColumns+bookFrom+`
		WHERE p.user_id IS NOT NULL AND NOT p.finished AND b.status = 'ok' AND b.library_id = ANY($3)
		ORDER BY p.updated_at DESC LIMIT $2`, userID, limit, libraryIDs)
}

func (d *DB) RecentlyAdded(ctx context.Context, userID int64, libraryIDs []int64, limit int) ([]Book, error) {
	return d.queryBooks(ctx, `SELECT `+bookColumns+bookFrom+`
		WHERE b.status = 'ok' AND b.library_id = ANY($3) ORDER BY b.added_at DESC, b.id DESC LIMIT $2`, userID, limit, libraryIDs)
}

// BookByPath finds the book currently recorded at an absolute path.
func (d *DB) BookByPath(ctx context.Context, path string) (Book, error) {
	b, err := scanBook(d.QueryRowContext(ctx, `SELECT `+bookColumns+bookFrom+` WHERE b.path = $2 ORDER BY b.id LIMIT 1`, 0, path))
	return b, notFound(err)
}

func (d *DB) BookByHash(ctx context.Context, hash string) (Book, error) {
	b, err := scanBook(d.QueryRowContext(ctx, `SELECT `+bookColumns+bookFrom+` WHERE b.content_hash = $2`, 0, hash))
	return b, notFound(err)
}

// NewBook is what the scanner inserts.
type NewBook struct {
	LibraryID   int64
	SeriesID    int64
	ContentHash string
	Path        string
	Format      string
	Title       string
	Author      string
	PageCount   int
	FileSize    int64
	Mtime       int64
	Status      string
	ErrorMsg    string
	Pages       []string // comic page entry names, in reading order
}

// InsertBook adds a book and its comic page list in one transaction.
func (d *DB) InsertBook(ctx context.Context, nb NewBook) (int64, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := Now()
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO books (library_id, series_id, content_hash, path, format, title, author, page_count,
		                   file_size, mtime, status, error_msg, added_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) RETURNING id`,
		nb.LibraryID, nb.SeriesID, nb.ContentHash, nb.Path, nb.Format, nb.Title, nb.Author, nb.PageCount,
		nb.FileSize, nb.Mtime, nb.Status, nb.ErrorMsg, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	if err := insertPages(ctx, tx, id, nb.Pages); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func insertPages(ctx context.Context, tx *sql.Tx, bookID int64, pages []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM comic_pages WHERE book_id = $1`, bookID); err != nil {
		return err
	}
	if len(pages) == 0 {
		return nil
	}
	idx := make([]int32, len(pages))
	for i := range pages {
		idx[i] = int32(i)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO comic_pages (book_id, page_index, entry_name)
		SELECT $1, i, n FROM unnest($2::int[], $3::text[]) AS t(i, n)`, bookID, idx, pages)
	return err
}

// MoveBook records a new location for a known book (same content hash).
func (d *DB) MoveBook(ctx context.Context, id, libraryID, seriesID int64, path string, mtime int64) error {
	_, err := d.ExecContext(ctx, `
		UPDATE books SET library_id = $2, series_id = $3, path = $4, mtime = $5,
		       status = CASE WHEN status = 'missing' THEN 'ok' ELSE status END, updated_at = $6
		WHERE id = $1`, id, libraryID, seriesID, path, mtime, Now())
	return err
}

// MarkMissing flags books as missing (their progress and notes stay).
func (d *DB) MarkMissing(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE books SET status = 'missing', updated_at = $2 WHERE id = ANY($1) AND status <> 'missing'`, ids, Now())
	return err
}

// PathIndex is the minimal per-book data the scanner needs to skip unchanged files.
type PathIndex struct {
	ID     int64
	Size   int64
	Mtime  int64
	Status string
	Path   string
}

// BooksUnder lists books whose path is below a root folder (any library).
func (d *DB) BooksUnder(ctx context.Context, root string) (map[string]PathIndex, error) {
	prefix := root
	if prefix != "/" {
		prefix += "/"
	}
	rows, err := d.QueryContext(ctx, `SELECT id, path, file_size, mtime, status FROM books WHERE starts_with(path, $1)`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PathIndex{}
	for rows.Next() {
		var p PathIndex
		if err := rows.Scan(&p.ID, &p.Path, &p.Size, &p.Mtime, &p.Status); err != nil {
			return nil, err
		}
		out[p.Path] = p
	}
	return out, rows.Err()
}

// ComicPageEntry returns the archive entry name of a 0-based page.
func (d *DB) ComicPageEntry(ctx context.Context, bookID int64, index int) (string, error) {
	var name string
	err := d.QueryRowContext(ctx, `SELECT entry_name FROM comic_pages WHERE book_id = $1 AND page_index = $2`, bookID, index).Scan(&name)
	return name, notFound(err)
}

// BooksWithoutCover lists ids of present books that still need a thumbnail.
func (d *DB) BooksWithoutCover(ctx context.Context) ([]int64, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT b.id FROM books b LEFT JOIN covers c ON c.book_id = b.id
		WHERE c.book_id IS NULL AND b.status = 'ok' ORDER BY b.added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (d *DB) SaveCover(ctx context.Context, bookID int64, etag string, img []byte) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO covers (book_id, etag, image, created_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (book_id) DO UPDATE SET etag = EXCLUDED.etag, image = EXCLUDED.image, created_at = EXCLUDED.created_at`,
		bookID, etag, img, Now())
	return err
}

func (d *DB) Cover(ctx context.Context, bookID int64) (etag string, img []byte, err error) {
	err = d.QueryRowContext(ctx, `SELECT etag, image FROM covers WHERE book_id = $1`, bookID).Scan(&etag, &img)
	return etag, img, notFound(err)
}

// SetBookError records that a book could not be read (e.g. a corrupt archive).
func (d *DB) SetBookError(ctx context.Context, id int64, msg string) error {
	_, err := d.ExecContext(ctx, `UPDATE books SET status = 'error', error_msg = $2, updated_at = $3 WHERE id = $1`, id, msg, Now())
	return err
}
