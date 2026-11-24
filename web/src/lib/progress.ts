// Reading-progress sync: saves on page change, debounced saves while scrolling,
// a sendBeacon flush when the page is hidden, and an IndexedDB queue (latest
// position per book only) for saves that failed, retried with backoff.
import { deviceId } from './device';
import { idb as idbStore, PROGRESS_STORE } from './idb';
import type { Progress } from './types';

const idb = <T>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest<T>) => idbStore<T>(PROGRESS_STORE, mode, f);

export interface Position { page: number; offset: number }
interface Pending { bookId: number; body: SaveBody }
interface SaveBody { page: number; offset: number; client_ts: number; device: string }

async function queuePut(p: Pending) {
  try {
    const existing = await idb<Pending | undefined>('readonly', (s) => s.get(p.bookId));
    if (existing && existing.body.client_ts >= p.body.client_ts) return;
    await idb('readwrite', (s) => s.put(p));
  } catch (e) { console.warn('progress queue unavailable', e); }
}

async function queueDelete(bookId: number, upTo: number) {
  try {
    const existing = await idb<Pending | undefined>('readonly', (s) => s.get(bookId));
    if (existing && existing.body.client_ts <= upTo) await idb('readwrite', (s) => s.delete(bookId));
  } catch {}
}

async function queueAll(): Promise<Pending[]> {
  try { return await idb<Pending[]>('readwrite', (s) => s.getAll()); } catch { return []; }
}

/** The latest queued (unsent) position for a book, if any. */
export async function pendingFor(bookId: number): Promise<SaveBody | null> {
  try { return (await idb<Pending | undefined>('readonly', (s) => s.get(bookId)))?.body ?? null; } catch { return null; }
}

async function send(bookId: number, body: SaveBody): Promise<Progress> {
  const res = await fetch(`/api/books/${bookId}/progress`, {
    method: 'PUT',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    keepalive: true,
  });
  if (!res.ok) {
    const err = new Error(`save failed: ${res.status}`) as Error & { status: number };
    err.status = res.status;
    throw err;
  }
  return res.json();
}

let flushing = false;
let retryTimer: number | undefined;
let retryDelay = 2000;

/** Send everything in the queue. Retries with exponential backoff on failure. */
export async function flushQueue(): Promise<void> {
  if (flushing) return;
  flushing = true;
  clearTimeout(retryTimer);
  let failed = false;
  try {
    for (const p of await queueAll()) {
      try {
        await send(p.bookId, p.body);
        await queueDelete(p.bookId, p.body.client_ts);
      } catch (e: any) {
        // 4xx other than auth/timeouts will never succeed: drop it.
        if (e?.status && e.status >= 400 && e.status < 500 && e.status !== 401 && e.status !== 408 && e.status !== 429) {
          await queueDelete(p.bookId, p.body.client_ts);
        } else failed = true;
      }
    }
  } finally {
    flushing = false;
  }
  if (failed) {
    retryTimer = window.setTimeout(flushQueue, retryDelay);
    retryDelay = Math.min(retryDelay * 2, 60000);
  } else retryDelay = 2000;
}

// This device's last position per book, so "jump to the other device" is only
// offered when the server is ahead of what this device last read.
const LOCAL_KEY = (id: number) => `bookman:pos:${id}`;

export function localPosition(bookId: number): (Position & { ts: number }) | null {
  try {
    const v = localStorage.getItem(LOCAL_KEY(bookId));
    return v ? JSON.parse(v) : null;
  } catch { return null; }
}

function setLocalPosition(bookId: number, pos: Position, ts: number) {
  try { localStorage.setItem(LOCAL_KEY(bookId), JSON.stringify({ ...pos, ts })); } catch {}
}

export class ProgressSync {
  private last: Position | null = null;
  private lastSent: string = '';
  private timer: number | undefined;
  private onHide = () => { if (document.visibilityState === 'hidden') this.beacon(); };
  private onPageHide = () => this.beacon();
  onSaved: (p: Progress) => void = () => {};

  constructor(private bookId: number) {
    document.addEventListener('visibilitychange', this.onHide);
    window.addEventListener('pagehide', this.onPageHide);
  }

  /** Record where the book was opened, without saving it: just opening must not overwrite another device. */
  prime(pos: Position) {
    this.last = { ...pos };
    this.lastSent = `${pos.page}:${Math.round(pos.offset * 10000) / 10000}`;
  }

  /** Report the current position. A page change saves at once; scrolling within a page is debounced to 2 s. */
  update(pos: Position) {
    const pageChanged = !this.last || this.last.page !== pos.page;
    this.last = { page: pos.page, offset: Math.min(1, Math.max(0, pos.offset)) };
    const key = `${this.last.page}:${Math.round(this.last.offset * 10000) / 10000}`;
    if (key === this.lastSent && this.timer === undefined) return;
    setLocalPosition(this.bookId, this.last, Date.now());
    if (pageChanged) {
      this.saveNow();
    } else if (this.timer === undefined) {
      this.timer = window.setTimeout(() => this.saveNow(), 2000);
    }
  }

  private body(): SaveBody | null {
    if (!this.last) return null;
    return { page: this.last.page, offset: Math.round(this.last.offset * 10000) / 10000, client_ts: Date.now(), device: deviceId() };
  }

  async saveNow() {
    clearTimeout(this.timer);
    this.timer = undefined;
    const body = this.body();
    if (!body) return;
    const key = `${body.page}:${body.offset}`;
    if (key === this.lastSent) return;
    this.lastSent = key;
    try {
      const stored = await send(this.bookId, body);
      await queueDelete(this.bookId, body.client_ts);
      this.onSaved(stored);
    } catch {
      this.lastSent = '';
      await queuePut({ bookId: this.bookId, body });
      flushQueue();
    }
  }

  /** Flush with sendBeacon (survives the tab closing). Also queued in case the beacon is lost. */
  beacon() {
    clearTimeout(this.timer);
    this.timer = undefined;
    const body = this.body();
    if (!body) return;
    const key = `${body.page}:${body.offset}`;
    if (key === this.lastSent) return;
    const blob = new Blob([JSON.stringify(body)], { type: 'text/plain' });
    const ok = navigator.sendBeacon?.(`/api/books/${this.bookId}/progress`, blob);
    if (ok) this.lastSent = key;
    // Keep a copy until a later save confirms it: a re-send is harmless (same client_ts loses).
    queuePut({ bookId: this.bookId, body });
  }

  destroy() {
    this.beacon();
    document.removeEventListener('visibilitychange', this.onHide);
    window.removeEventListener('pagehide', this.onPageHide);
  }
}
