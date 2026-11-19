# Folio — self-hosted book & comic server

Working name: **Folio** (rename freely). A "Jellyfin for books": point it at folders of PDFs and comics, browse the library in a web UI, read in the browser with a notes side panel, and never lose reading progress.

## Instructions for Claude Code

- Build in the milestones below, in order. Finish each one with passing tests and a working binary before starting the next. Commit at the end of each milestone.
- Ask before adding any dependency not listed here.
- Keep everything **cgo-free** (`CGO_ENABLED=0`). The output is a single static binary with the UI embedded.
- Prefer the standard library. Keep the code boring and readable.
- Put the dev commands (`make dev`, `make build`, `make test`) in a Makefile and document them in README.md.

## Hard requirements

1. **Single static binary.** The Go server embeds the built frontend via `//go:embed`. No external runtime dependencies.
2. **Formats:** PDF, CBZ, CBR (including RAR5).
3. **Reading progress is the most important feature.** It must survive tab closes, phone locks, network drops, window resizes, zoom changes and device switches.
4. **Notes** attached to books: page-level, optionally anchored to a text selection (PDF only).
5. **The library on disk is read-only.** Never modify book files. All state lives in SQLite.

## Stack

**Backend (Go 1.23+)**
- HTTP: `net/http` with the 1.22+ routing patterns. `chi` only if middleware becomes painful.
- DB: `modernc.org/sqlite` (pure Go), plain SQL with embedded migration files. No ORM.
- PDF: `github.com/klippa-app/go-pdfium` in **WebAssembly mode** (wazero) for page count, metadata and cover rendering.
- CBZ: `archive/zip`.
- CBR: `github.com/nwaples/rardecode/v2`.
- Images: `golang.org/x/image/draw` for resizing; thumbnails as JPEG (or WebP if a pure-Go encoder is acceptable).
- Watching: `github.com/fsnotify/fsnotify`.
- Passwords: `golang.org/x/crypto/argon2` (argon2id).
- Logging: `log/slog`.

**Frontend**
- Svelte + Vite + TypeScript, built into `web/dist`, embedded by Go.
- `pdfjs-dist`, using its `PDFViewer` component from `pdfjs-dist/web/pdf_viewer` (not the stock viewer in an iframe). Bundle the PDF.js worker locally, with no CDN.
- No CSS framework required; plain CSS with variables, dark and light themes.

## Architecture

```
cmd/folio/            main, flags/config
internal/config/      config file + env overrides
internal/db/          sqlite open, migrations, queries
internal/library/     scanner, watcher, book import
internal/formats/     BookSource interface + pdf, cbz, cbr implementations
internal/thumbs/      cover generation worker + on-disk cache
internal/api/         HTTP handlers, auth middleware
internal/auth/        users, sessions
web/                  Svelte app
```

### Format abstraction

```go
type BookSource interface {
    Format() string                 // "pdf" | "cbz" | "cbr"
    PageCount() (int, error)
    Metadata() (Metadata, error)    // title, author, etc. where available
    Cover(maxW, maxH int) (image.Image, error)
    // Comics only: raw page image bytes + content type.
    Page(n int) ([]byte, string, error)
}
```

- **PDF:** `Page()` is unsupported. The browser renders pages with PDF.js from the raw file.
- **CBZ/CBR:** pages are the archive's image entries (jpg/jpeg/png/webp/gif), sorted with **natural sort** by path. Skip `__MACOSX`, dotfiles and non-images.
- **CBR:** rardecode reads sequentially. On import, record the ordered list of page entry names. To serve page N, stream until that entry. Add a small LRU cache of extracted pages (size configurable) so page-turning doesn't re-read the archive. Handle solid archives correctly.

### Library scanning

- Config lists one or more library roots. On startup: full scan with `filepath.WalkDir`. After that, use fsnotify for changes, plus a periodic rescan (default 6h) as a safety net.
- Identify books by **content hash** (xxhash or sha256 of the first 1 MiB + file size + full-file size), not by path. A moved or renamed file keeps its progress and notes. Store the path as mutable.
- Group books into **series** by parent folder by default.
- Missing files are marked `missing`, not deleted, so their progress and notes survive.
- Cover generation runs in a bounded worker pool (default `runtime.NumCPU()/2`, min 1). Covers are cached on disk under the data dir, keyed by book id.

## Data model (SQLite)

```sql
users(id, username UNIQUE, password_hash, is_admin, created_at)
sessions(token_hash PRIMARY KEY, user_id, expires_at, created_at)

libraries(id, name, root_path UNIQUE)
series(id, library_id, name, path)
books(id, library_id, series_id, content_hash UNIQUE, path, format,
      title, author, page_count, file_size, mtime,
      status,            -- 'ok' | 'missing' | 'error'
      error_msg, added_at, updated_at)
comic_pages(book_id, page_index, entry_name, PRIMARY KEY(book_id, page_index))

progress(user_id, book_id,
         page INTEGER,          -- 1-based
         offset REAL,           -- 0..1 fraction within page, NOT pixels
         percent REAL,          -- (page-1+offset)/page_count
         finished BOOLEAN,
         client_ts INTEGER,     -- ms epoch from client; used for conflict resolution
         device TEXT,
         updated_at INTEGER,
         PRIMARY KEY(user_id, book_id))

reading_sessions(id, user_id, book_id, device,
                 start_page, end_page, started_at, ended_at)   -- append-only

notes(id, user_id, book_id, page,
      body_md TEXT,
      quote TEXT NULL,          -- selected text (PDF)
      anchor_start INTEGER NULL, anchor_end INTEGER NULL,  -- char offsets in page text layer
      color TEXT NULL,
      created_at, updated_at, deleted_at NULL)
```

Use WAL mode, `foreign_keys=ON` and `busy_timeout`.

## Reading progress — exact behaviour

**Client**
- Track the current `page` plus `offset` (the fraction of the page scrolled past the viewport top, 0–1). Comics always use offset 0.
- Save on every page change, and debounce saves to every 2s while scrolling.
- On `visibilitychange` (hidden) and `pagehide`, flush with `navigator.sendBeacon`.
- If a save fails, queue it in IndexedDB and retry with backoff. Flush on `online` and on app load. Only the latest pending position per book matters.
- Every save includes `client_ts` (ms) and a stable `device` id (random, stored in localStorage).

**Server**
- `PUT /api/books/{id}/progress` upserts **only if** `client_ts` > stored `client_ts` (last write wins by client time). It returns the stored row either way.
- It auto-sets `finished=true` when the last page is reached with offset ≥ 0.9. The user can mark finished or unfinished manually.
- It maintains `reading_sessions`: a save more than 10 minutes after the device's last save for that book opens a new session; otherwise it extends the current one.

**Resume**
- When opening a book, fetch progress. Restore the position **after** the target page's `pagerendered` event (PDF) or after the image loads (comics), not before.
- If the server's progress came from a different device and is ahead of this device's last local position, show a non-blocking prompt: "You were on page X on <device> — jump there?"
- Offsets are fractions, so the position survives resizing the notes panel and zoom changes. Re-apply after any viewer resize.

## Reader UI

- A split layout: viewer on the left, notes panel on the right. The divider is draggable, the panel width is remembered per device, and a toggle collapses the panel (shortcut `n`).
- On viewports narrower than 900px, the notes panel becomes a bottom drawer.
- **PDF viewer:** continuous vertical scroll, fit-width by default (re-apply `currentScaleValue = 'page-width'` when the split changes), zoom controls, page-number input, text layer enabled for selection, and in-document search via PDF.js find.
- **Comic viewer:** single-page and continuous-scroll modes; keyboard and arrow navigation, click/tap zones on left and right, preload the next 2 pages.
- Show a progress bar with the percent at the bottom.
- Keyboard: arrows / PageUp / PageDown / space navigate, `n` toggles notes, `Esc` returns to the library.

### Notes panel
- Tabs: **This page** (follows scrolling) and **All**.
- Select text in a PDF, then use the floating "Add note" button to create a note with `quote` + anchor offsets. Render saved anchors as highlights on the text layer.
- Notes use a Markdown editor (a plain textarea with a preview toggle is fine) and autosave with a debounce. Deleting is a soft delete with undo.
- Clicking a note jumps to its page and flashes the highlight.
- Export all of a book's notes as a Markdown file.

## Library UI

- Home: **Continue reading** (by `progress.updated_at`, unfinished), **Recently added**, then libraries.
- Library view: a grid of covers with a progress overlay; filter by format and status (unread / reading / finished); sort by title, added or last read; search by title and author.
- Series view: the series' books in natural sort order.
- Book detail: cover, metadata, progress, reading history (from `reading_sessions`), notes count, and buttons for Read / Mark finished / Reset progress.

## HTTP API (JSON, under `/api`)

```
POST   /auth/login            POST /auth/logout       GET /auth/me
GET    /libraries             GET  /libraries/{id}/books?q=&format=&status=&sort=
GET    /series/{id}
GET    /books/{id}            GET  /books/{id}/cover  (cached image, ETag)
GET    /books/{id}/file       (raw file; must support Range via http.ServeContent)
GET    /books/{id}/pages/{n}  (comics; image bytes, long cache headers)
GET    /books/{id}/progress   PUT  /books/{id}/progress
POST   /books/{id}/finished   DELETE /books/{id}/progress
GET    /books/{id}/sessions
GET    /books/{id}/notes      POST /books/{id}/notes
PATCH  /notes/{id}            DELETE /notes/{id}
GET    /books/{id}/notes/export.md
GET    /continue-reading      GET /recently-added
POST   /admin/scan            (admin) GET/POST /admin/users
```

- Auth: session cookie (`HttpOnly`, `SameSite=Lax`, `Secure` when behind TLS), with the token stored hashed. On first run, if no users exist, the UI shows a create-admin screen.
- `sendBeacon` can't set headers, so the progress endpoint must accept the cookie with a `text/plain` JSON body. Protect it against CSRF by checking `Origin`.
- Return proper errors as `{ "error": "..." }`.

## Configuration

`folio.toml` (path set by flag), with env var overrides (`FOLIO_*`):

```toml
listen = ":8080"
data_dir = "/var/lib/folio"        # sqlite db + cover cache
libraries = [{ name = "Books", path = "/media/books" }, { name = "Comics", path = "/media/comics" }]
rescan_interval = "6h"
thumb_workers = 2
page_cache_mb = 128
```

Use a small TOML parser (`github.com/BurntSushi/toml`) — that is the one other allowed dependency.

## Milestones

1. **Skeleton:** config, SQLite and migrations, embedded Svelte "hello" page, Makefile, a single static binary (`CGO_ENABLED=0`), and a Dockerfile `FROM scratch`.
2. **Library:** the `BookSource` interface plus PDF/CBZ/CBR implementations, the scanner, content hashing, the cover worker, and the watcher. Unit tests use small fixture files generated in tests, or committed fixtures under 100 KB each.
3. **Auth + API:** users, sessions, first-run admin, and the library/book/cover/file/page endpoints.
4. **Library UI:** home, library grid, series, book detail.
5. **Reader:** PDF viewer with fit-width and resize handling, plus the comic viewer.
6. **Progress:** everything in "Reading progress — exact behaviour", including beacon, offline queue, conflict handling, sessions and the resume prompt. Add tests for the upsert/conflict logic and session splitting, plus a Playwright test that closes the tab mid-book and verifies resume.
7. **Notes:** panel, text-anchored highlights, autosave, Markdown export.
8. **Polish:** keyboard shortcuts, mobile layout, dark mode, empty/error states, README with deployment (systemd unit + Docker + Kubernetes manifest example).

## Non-goals for v1

EPUB, OPDS, metadata scraping from the internet, editing book files, transcoding, and multiple-library permissions per user. Design so EPUB (a new `BookSource`) and OPDS (new routes) can be added later without schema rewrites.
