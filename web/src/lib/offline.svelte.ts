// Offline support, page side. The service worker (web/sw.js) serves the app and
// the last good API answers when the server is unreachable; this module
// registers it, saves books for reading with the server down, and keeps that
// within the space the user allows (Settings → Offline).
//
// Storage layout in the "bookman-books" cache (read by web/sw.js):
//   /offline/books/{id}/meta     JSON { size, type, chunk, chunks, pages }; present only once complete
//   /offline/books/{id}/{n}      PDF bytes, CHUNK bytes per entry (so nothing big is ever held in memory)
//   /api/books/{id}/pages/{n}    comic pages, under their own URLs
// The list of saved books (titles, sizes, last opened) is kept in localStorage.
import type { Book } from './types';

export const BOOKS = 'bookman-books';
const API = 'bookman-api';
const MEDIA = 'bookman-media';
const SAVED_KEY = 'bookman:saved-books';
const SETTINGS_KEY = 'bookman:offline-settings';
const CHUNK = 1024 * 1024;
// Never plan to use more than this share of what the browser allows the site.
const QUOTA_SHARE = 0.6;
// Leave this much of the quota free when saving.
const HEADROOM = 64 * 1024 * 1024;
const GB = 1024 ** 3;

export const metaKey = (id: number) => `/offline/books/${id}/meta`;
const chunkKey = (id: number, n: number) => `/offline/books/${id}/${n}`;
const pageKey = (id: number, n: number) => `/api/books/${id}/pages/${n}`;

export interface OfflineSettings { autoSave: boolean; maxBytes: number }
export interface SavedBook { id: number; title: string; format: string; size: number; at: number }

function loadSettings(): OfflineSettings {
  const def = { autoSave: true, maxBytes: 2 * GB };
  try { return { ...def, ...JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}') }; } catch { return def; }
}

export const connection = $state({ offline: false });
export const offline = $state({
  supported: typeof caches !== 'undefined' && 'serviceWorker' in navigator,
  settings: loadSettings(),
  // The book being saved now.
  saving: null as null | { id: number; title: string; done: number; total: number },
  // Why the last book couldn't be saved, if it couldn't.
  lastError: '',
  lastErrorBook: 0,
});

export function saveSettings(s: OfflineSettings) {
  offline.settings = s;
  try { localStorage.setItem(SETTINGS_KEY, JSON.stringify(s)); } catch {}
  enforceBudget().catch(() => {});
}

export function registerServiceWorker() {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return;
  navigator.serviceWorker.register('/sw.js').catch((e) => console.warn('service worker not registered', e));
  // Ask the browser not to evict saved books under storage pressure.
  navigator.storage?.persist?.().catch(() => {});
  // Remove leftovers of downloads that were cut off, once things are quiet.
  setTimeout(() => cleanOrphans().catch(() => {}), 30000);
}

export function savedBooks(): SavedBook[] {
  try { return JSON.parse(localStorage.getItem(SAVED_KEY) ?? '[]'); } catch { return []; }
}
function setSaved(l: SavedBook[]) {
  try { localStorage.setItem(SAVED_KEY, JSON.stringify(l)); } catch {}
}

export async function isSaved(id: number): Promise<boolean> {
  try { return !!(await (await caches.open(BOOKS)).match(metaKey(id))); } catch { return false; }
}

/** Space the saved books may use: the user's setting, capped by what the browser allows. */
export async function budget(): Promise<number> {
  const est = await navigator.storage?.estimate?.().catch(() => undefined);
  const cap = est?.quota ? est.quota * QUOTA_SHARE : Infinity;
  return Math.min(offline.settings.maxBytes, cap);
}

// One book is saved at a time; opening several queues them, newest first.
const queue: Book[] = [];
let running = false;
const cancelled = new Set<number>();

// Saving and cleaning up hold this lock, so one tab never deletes chunks that
// another tab is still writing (and two tabs don't download at once).
const LOCK = 'bookman-offline-books';
async function withLock<T>(f: () => Promise<T>, ifAvailable = false): Promise<T | undefined> {
  if (!navigator.locks) return f();
  return navigator.locks.request(LOCK, { ifAvailable }, (lock) => (lock ? f() : undefined));
}

/** Save a book for offline reading in the background (if enabled), or touch it if already saved. */
export function saveBook(book: Book, opts: { force?: boolean } = {}) {
  if (!offline.supported || !navigator.serviceWorker?.controller) return;
  if (!opts.force && !offline.settings.autoSave) return;
  // Respect the browser's data saver unless asked explicitly.
  if (!opts.force && (navigator as any).connection?.saveData) return;
  cancelled.delete(book.id);
  const i = queue.findIndex((b) => b.id === book.id);
  if (i >= 0) queue.splice(i, 1);
  queue.unshift(book);
  if (!running) run();
}

async function run() {
  running = true;
  try {
    await withLock(drain);
  } finally {
    running = false;
  }
}

async function drain() {
  while (queue.length) {
    const b = queue.shift()!;
    try {
      await saveOne(b);
      offline.lastError = '';
    } catch (e: any) {
      if (!cancelled.has(b.id)) {
        offline.lastError = `“${b.title}” wasn't saved for offline reading: ${e?.message ?? e}`;
        offline.lastErrorBook = b.id;
      }
      setSaved(savedBooks().filter((s) => s.id !== b.id));
      await deleteStored(b).catch(() => {});
    } finally {
      offline.saving = null;
    }
  }
}

async function saveOne(book: Book) {
  const cache = await caches.open(BOOKS);
  const list = savedBooks().filter((s) => s.id !== book.id);
  const entry: SavedBook = { id: book.id, title: book.title, format: book.format, size: book.file_size, at: Date.now() };
  if (await cache.match(metaKey(book.id))) {
    setSaved([entry, ...list]); // already saved: just mark it recently opened
    return;
  }

  // Make room first: drop the least recently opened books beyond the budget.
  const limit = await budget();
  if (book.file_size > limit) throw new Error(`it is larger than the offline space (${fmt(limit)}); raise it in Settings`);
  let used = book.file_size;
  const keep: SavedBook[] = [];
  for (const s of list) {
    if (used + s.size <= limit) { used += s.size; keep.push(s); } else await deleteStored(s);
  }
  setSaved(keep);
  const est = await navigator.storage?.estimate?.().catch(() => undefined);
  if (est?.quota && est.usage !== undefined && est.quota - est.usage < book.file_size + HEADROOM) {
    throw new Error('not enough free space on this device');
  }

  offline.saving = { id: book.id, title: book.title, done: 0, total: book.file_size };
  // Record it before downloading, so an interrupted download is cleaned up later.
  setSaved([{ ...entry, at: 0 }, ...keep]);
  let meta: object;
  if (book.format === 'pdf') meta = await savePdf(cache, book);
  else meta = await saveComic(cache, book);
  check(book.id);
  await cache.put(metaKey(book.id), new Response(JSON.stringify(meta), { headers: { 'Content-Type': 'application/json' } }));
  setSaved([entry, ...savedBooks().filter((s) => s.id !== book.id)]);
}

function check(id: number) {
  if (cancelled.has(id)) throw new Error('cancelled');
}

async function savePdf(cache: Cache, book: Book) {
  // The header tells the service worker to let this go straight to the server.
  const res = await fetch(`/api/books/${book.id}/file`, { credentials: 'same-origin', headers: { 'X-Bookman-Save': '1' }, priority: 'low' } as RequestInit);
  if (res.status !== 200 || !res.body) throw new Error(res.status ? `server answered ${res.status}` : 'server unreachable');
  const type = res.headers.get('Content-Type') || 'application/pdf';
  const reader = res.body.getReader();
  let buf = new Uint8Array(CHUNK), fill = 0, n = 0, size = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      check(book.id);
      for (let off = 0; off < value.length;) {
        const take = Math.min(CHUNK - fill, value.length - off);
        buf.set(value.subarray(off, off + take), fill);
        fill += take; off += take;
        if (fill === CHUNK) {
          await cache.put(chunkKey(book.id, n++), new Response(buf));
          buf = new Uint8Array(CHUNK); fill = 0;
        }
      }
      size += value.length;
      if (offline.saving) offline.saving.done = size;
    }
  } catch (e) {
    reader.cancel().catch(() => {});
    throw e;
  }
  if (fill) await cache.put(chunkKey(book.id, n++), new Response(buf.slice(0, fill)));
  return { size, type, chunk: CHUNK, chunks: n };
}

async function saveComic(cache: Cache, book: Book) {
  let size = 0;
  for (let i = 1; i <= book.page_count; i++) {
    check(book.id);
    const key = pageKey(book.id, i);
    const have = await cache.match(key);
    if (have) { size += Number(have.headers.get('Content-Length') || 0); continue; }
    const res = await fetch(key, { credentials: 'same-origin', headers: { 'X-Bookman-Save': '1' }, priority: 'low' } as RequestInit);
    if (res.status !== 200) throw new Error(res.status ? `server answered ${res.status}` : 'server unreachable');
    size += Number(res.headers.get('Content-Length') || 0);
    await cache.put(key, res);
    if (offline.saving) offline.saving.done = Math.round((book.file_size * i) / book.page_count);
  }
  return { size, pages: book.page_count };
}

/** Delete a saved book (or stop saving it). */
export async function removeBook(id: number) {
  cancelled.add(id);
  const i = queue.findIndex((b) => b.id === id);
  if (i >= 0) queue.splice(i, 1);
  const s = savedBooks().find((x) => x.id === id);
  setSaved(savedBooks().filter((x) => x.id !== id));
  await deleteStored(s ?? { id });
}

async function deleteStored(b: { id: number }) {
  const cache = await caches.open(BOOKS);
  await cache.delete(metaKey(b.id));
  // Delete by prefix: covers partial downloads whose size isn't known.
  const prefixes = [`/offline/books/${b.id}/`, `/api/books/${b.id}/pages/`];
  for (const req of await cache.keys()) {
    const p = new URL(req.url).pathname;
    if (prefixes.some((x) => p.startsWith(x))) await cache.delete(req);
  }
}

export async function removeAllBooks() {
  for (const b of queue) cancelled.add(b.id);
  if (offline.saving) cancelled.add(offline.saving.id);
  queue.length = 0;
  setSaved([]);
  await withLock(() => emptyCache(BOOKS));
}

// Deletes entry by entry: Chrome frees the space of deleted entries at once,
// while a deleted cache can keep its files until every handle to it is gone.
async function emptyCache(name: string) {
  if (!(await caches.has(name))) return;
  const cache = await caches.open(name);
  for (const req of await cache.keys()) await cache.delete(req);
}

/** Apply a lowered limit at once. */
async function enforceBudget() {
  const limit = await budget();
  let used = 0;
  const keep: SavedBook[] = [];
  for (const s of savedBooks().sort((a, b) => b.at - a.at)) {
    if (used + s.size <= limit) { used += s.size; keep.push(s); } else await deleteStored(s);
  }
  setSaved(keep);
}

/** Drop stored books missing from the list, and list entries whose download never finished. */
function cleanOrphans() {
  if (!offline.supported || running) return Promise.resolve();
  return withLock(cleanOrphansLocked, true);
}

async function cleanOrphansLocked() {  const cache = await caches.open(BOOKS);
  const known = new Set<number>();
  const list: SavedBook[] = [];
  for (const s of savedBooks()) {
    if (await cache.match(metaKey(s.id))) { known.add(s.id); list.push(s); }
  }
  setSaved(list);
  for (const req of await cache.keys()) {
    const m = /^\/(?:offline\/books|api\/books)\/(\d+)\//.exec(new URL(req.url).pathname);
    if (m && !known.has(Number(m[1]))) await cache.delete(req);
  }
}

/** Empty the cached API answers and covers (saved books stay). */
export async function clearPageCache() {
  await Promise.all([emptyCache(API), emptyCache(MEDIA)]);
}

/** On sign-out: drop the cached API answers (they belong to this user). Saved books stay. */
export async function clearUserData() {
  try { await emptyCache(API); } catch {}
}

function fmt(n: number) {
  return n >= GB ? `${(n / GB).toFixed(1)} GB` : `${Math.round(n / 1024 ** 2)} MB`;
}
