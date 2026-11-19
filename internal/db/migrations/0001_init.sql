-- All timestamps are milliseconds since the Unix epoch (BIGINT), like the
-- client_ts the browser sends, so the API never converts between formats.

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    BIGINT NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

CREATE TABLE libraries (
    id        BIGSERIAL PRIMARY KEY,
    name      TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE
);

CREATE TABLE series (
    id         BIGSERIAL PRIMARY KEY,
    library_id BIGINT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,          -- relative to the library root, "." for the root
    UNIQUE (library_id, path)
);

CREATE TABLE books (
    id           BIGSERIAL PRIMARY KEY,
    library_id   BIGINT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    series_id    BIGINT REFERENCES series(id) ON DELETE SET NULL,
    content_hash TEXT NOT NULL UNIQUE,
    path         TEXT NOT NULL,         -- absolute, mutable (moves keep the row)
    format       TEXT NOT NULL,         -- pdf | cbz | cbr (epub later)
    title        TEXT NOT NULL,
    author       TEXT NOT NULL DEFAULT '',
    page_count   INTEGER NOT NULL DEFAULT 0,
    file_size    BIGINT NOT NULL,
    mtime        BIGINT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'ok',   -- ok | missing | error
    error_msg    TEXT NOT NULL DEFAULT '',
    added_at     BIGINT NOT NULL,
    updated_at   BIGINT NOT NULL
);
CREATE INDEX books_library_idx ON books(library_id);
CREATE INDEX books_series_idx ON books(series_id);
CREATE INDEX books_path_idx ON books(path);
CREATE INDEX books_added_idx ON books(added_at DESC);

CREATE TABLE comic_pages (
    book_id    BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page_index INTEGER NOT NULL,        -- 0-based position in natural sort order
    entry_name TEXT NOT NULL,
    PRIMARY KEY (book_id, page_index)
);

-- Cover thumbnails (JPEG). Kept in the database instead of on disk so the pod
-- needs no volume of its own.
CREATE TABLE covers (
    book_id    BIGINT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    etag       TEXT NOT NULL,
    image      BYTEA NOT NULL,
    created_at BIGINT NOT NULL
);

CREATE TABLE progress (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id    BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page       INTEGER NOT NULL,          -- 1-based
    "offset"   DOUBLE PRECISION NOT NULL, -- 0..1 fraction within the page
    percent    DOUBLE PRECISION NOT NULL, -- (page-1+offset)/page_count
    finished   BOOLEAN NOT NULL DEFAULT FALSE,
    client_ts  BIGINT NOT NULL,           -- client clock, last write wins
    device     TEXT NOT NULL DEFAULT '',
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (user_id, book_id)
);
CREATE INDEX progress_user_updated_idx ON progress(user_id, updated_at DESC);

CREATE TABLE reading_sessions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id    BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    device     TEXT NOT NULL,
    start_page INTEGER NOT NULL,
    end_page   INTEGER NOT NULL,
    started_at BIGINT NOT NULL,
    ended_at   BIGINT NOT NULL
);
CREATE INDEX reading_sessions_lookup_idx ON reading_sessions(user_id, book_id, device, ended_at DESC);

CREATE TABLE notes (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id      BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page         INTEGER NOT NULL,
    body_md      TEXT NOT NULL DEFAULT '',
    quote        TEXT,
    anchor_start INTEGER,
    anchor_end   INTEGER,
    color        TEXT,
    created_at   BIGINT NOT NULL,
    updated_at   BIGINT NOT NULL,
    deleted_at   BIGINT
);
CREATE INDEX notes_user_book_idx ON notes(user_id, book_id);
