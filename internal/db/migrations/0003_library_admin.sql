-- Libraries are managed from the UI. The config file's list only seeds them on
-- first start (settings.libraries_seeded).
ALTER TABLE libraries ADD COLUMN exclude TEXT[] NOT NULL DEFAULT '{}';
-- Removing a library hides it but keeps its books, progress and notes; adding
-- the same folder again revives it.
ALTER TABLE libraries ADD COLUMN removed_at BIGINT;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
