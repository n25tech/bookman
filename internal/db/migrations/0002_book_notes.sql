-- Book notes (not tied to a page) are stored with page = 0.
COMMENT ON COLUMN notes.page IS '1-based page, or 0 for a book note';
