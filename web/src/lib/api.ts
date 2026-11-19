import type { AdminLibrary, Book, BrowseResult, FsListing, Library, Note, Progress, ReadingSession, Series, User } from './types';

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

// Called on any 401 so the app can show the login screen.
let onUnauthorized: () => void = () => {};
export function setUnauthorizedHandler(f: () => void) { onUnauthorized = f; }

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError(0, 'Network error: the server could not be reached.');
  }
  if (res.status === 401 && !path.startsWith('/auth/')) onUnauthorized();
  const text = await res.text();
  let data: any = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = text; }
  }
  if (!res.ok) throw new ApiError(res.status, (data && data.error) || res.statusText || 'Request failed');
  return data as T;
}

export const api = {
  status: () => request<{ setup_required: boolean }>('GET', '/auth/status'),
  me: () => request<User>('GET', '/auth/me'),
  login: (username: string, password: string) => request<User>('POST', '/auth/login', { username, password }),
  setup: (username: string, password: string) => request<User>('POST', '/auth/setup', { username, password }),
  logout: () => request<void>('POST', '/auth/logout'),

  libraries: () => request<Library[]>('GET', '/libraries'),
  libraryBooks: (id: number, params: Record<string, string>) =>
    request<{ library: Library; total: number; books: Book[] }>('GET', `/libraries/${id}/books?` + new URLSearchParams(params)),
  browse: (id: number, path: string) =>
    request<BrowseResult>('GET', `/libraries/${id}/browse?` + new URLSearchParams({ path })),
  librarySeries: (id: number) => request<Series[]>('GET', `/libraries/${id}/series`),
  series: (id: number) => request<{ series: Series; library: Library; books: Book[] }>('GET', `/series/${id}`),
  book: (id: number) => request<{ book: Book; library: Library; note_count: number }>('GET', `/books/${id}`),
  continueReading: () => request<Book[]>('GET', '/continue-reading'),
  recentlyAdded: () => request<Book[]>('GET', '/recently-added'),

  progress: (id: number) => request<Progress | null>('GET', `/books/${id}/progress`),
  setFinished: (id: number, finished: boolean) => request<Progress>('POST', `/books/${id}/finished`, { finished }),
  resetProgress: (id: number) => request<null>('DELETE', `/books/${id}/progress`),
  sessions: (id: number) => request<ReadingSession[]>('GET', `/books/${id}/sessions`),

  notes: (id: number) => request<Note[]>('GET', `/books/${id}/notes`),
  createNote: (id: number, n: Partial<Note>) => request<Note>('POST', `/books/${id}/notes`, n),
  updateNote: (id: number, patch: { body_md?: string; color?: string; page?: number; deleted?: boolean }) =>
    request<Note>('PATCH', `/notes/${id}`, patch),
  deleteNote: (id: number) => request<Note>('DELETE', `/notes/${id}`),

  scan: () => request<unknown>('POST', '/admin/scan'),
  scanStatus: () => request<{ scan: any; covers_pending: number }>('GET', '/admin/scan'),
  adminLibraries: () => request<{ libraries: AdminLibrary[]; media_roots: string[] }>('GET', '/admin/libraries'),
  createLibrary: (l: { name: string; path: string; exclude: string[] }) => request<AdminLibrary>('POST', '/admin/libraries', l),
  updateLibrary: (id: number, l: { name: string; path: string; exclude: string[] }) => request<AdminLibrary>('PATCH', `/admin/libraries/${id}`, l),
  removeLibrary: (id: number) => request<unknown>('DELETE', `/admin/libraries/${id}`),
  scanLibrary: (id: number) => request<unknown>('POST', `/admin/libraries/${id}/scan`),
  fs: (path: string) => request<FsListing>('GET', '/admin/fs?' + new URLSearchParams({ path })),
  users: () => request<User[]>('GET', '/admin/users'),
  createUser: (username: string, password: string, is_admin: boolean) =>
    request<User>('POST', '/admin/users', { username, password, is_admin }),
};

export function coverUrl(bookId: number, etag: string | null): string | null {
  return etag ? `/api/books/${bookId}/cover?v=${etag}` : null;
}
