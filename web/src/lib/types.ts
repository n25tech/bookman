export interface User { id: number; username: string; is_admin: boolean; created_at: number }
export interface Progress {
  book_id: number; page: number; offset: number; percent: number; finished: boolean;
  client_ts: number; device: string; updated_at: number;
}
export interface Book {
  id: number; library_id: number; series_id: number | null; series_name: string; rel_path: string;
  format: 'pdf' | 'cbz' | 'cbr'; title: string; author: string; page_count: number; file_size: number;
  status: 'ok' | 'missing' | 'error'; error_msg?: string; added_at: number; updated_at: number;
  cover_etag: string | null; progress: Progress | null;
}
export interface Library { id: number; name: string; book_count: number }
export interface Series {
  id: number; library_id: number; name: string; path: string; book_count: number;
  cover_book_id: number | null; cover_etag: string | null;
}
export interface ReadingSession { id: number; device: string; start_page: number; end_page: number; started_at: number; ended_at: number }
export interface Note {
  id: number; book_id: number; page: number; body_md: string; quote: string | null;
  anchor_start: number | null; anchor_end: number | null; color: string | null;
  created_at: number; updated_at: number; deleted_at?: number | null; client_id?: string | null;
}
export interface Folder {
  name: string; path: string; book_count: number; finished_count: number; reading_count: number;
  cover_book_id: number; cover_etag: string | null;
}
export interface BrowseResult {
  library: Library; path: string; breadcrumbs: { name: string; path: string }[]; folders: Folder[]; books: Book[];
}
export interface AdminLibrary { id: number; name: string; path: string; exclude: string[]; book_count: number; exists: boolean }
export interface FsEntry { name: string; path: string; library?: string }
export interface FsListing { path: string; parent: string | null; dirs: FsEntry[]; books_here?: number; library?: string }
