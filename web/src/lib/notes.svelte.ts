// Notes that survive the server being down. Every write is tried at once; if the
// server can't be reached it is queued in IndexedDB and the UI carries on with
// the local copy. The queue holds one entry per note with the state the note
// should end up in (not a list of edits), so it can be resent any number of
// times and always lands on the latest text. New notes carry a random client_id
// that the server uses to make a retried create return the existing note.
//
// Entries belong to the signed-in user and are only sent with their session. A
// write the server refuses for good (say the book was removed) is kept as
// "failed" and shown in Settings, so its text is never lost silently.
import { api, ApiError } from './api';
import { idb, NOTES_STORE } from './idb';
import type { Note } from './types';

type Field = 'body_md' | 'color' | 'deleted';
export interface NotePatch { body_md?: string; color?: string; deleted?: boolean }
export interface NewNote { page: number; body_md?: string; quote?: string | null; anchor_start?: number | null; anchor_end?: number | null; color?: string | null }

export interface PendingNote {
  key: string;            // the note's client_id, or "id:<server id>" for notes made before client ids
  userId: number;
  bookId: number;
  serverId: number | null; // null until the create has gone through
  create: boolean;         // still needs POST
  fields: Field[];         // what to send (a create also compares everything)
  note: Note;              // the state the note should end up in; what the UI shows
  ts: number;              // bumped on every change, so a send can tell it was overtaken
  failed?: string;         // refused by the server; kept for the user to retry or discard
}

export const notesSync = $state({ pending: 0, failed: 0 });

let userId: number | null = null;
/** Set by the app when the signed-in user changes. Starts sending that user's queue. */
export function setNotesUser(id: number | null) {
  userId = id;
  refreshCounts();
  if (id !== null) flushNotes();
}

type Listener = (bookId: number, localId: number, note: Note) => void;
const listeners = new Set<Listener>();
/** Called when a queued note reaches the server (its id may change from a temporary negative one). */
export function onNoteSynced(f: Listener): () => void {
  listeners.add(f);
  return () => listeners.delete(f);
}

// Temporary ids of notes made offline, and the server ids they got.
const resolved = new Map<number, number>();
let tempSeq = 0;
const tempId = () => -(Date.now() * 1000 + (tempSeq++ % 1000));

function newClientId(): string {
  if (crypto.randomUUID) return crypto.randomUUID(); // secure contexts only
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, '0')).join('');
}

const keyFor = (n: Note) => (n.client_id ? n.client_id : `id:${n.id}`);
const serverIdOf = (n: Note) => (n.id > 0 ? n.id : resolved.get(n.id) ?? null);

/** Network trouble or a server/proxy error: worth trying again later. */
export function retryable(e: unknown): boolean {
  if (!(e instanceof ApiError)) return true;
  return e.status === 0 || e.status === 401 || e.status === 408 || e.status === 429 || e.status >= 500;
}

function apply(n: Note, p: NotePatch): Note {
  const now = Date.now();
  const out = { ...n, updated_at: now };
  if (p.body_md !== undefined) out.body_md = p.body_md;
  if (p.color !== undefined) out.color = p.color || null;
  if (p.deleted !== undefined) out.deleted_at = p.deleted ? now : null;
  return out;
}

const getEntry = (key: string) => idb<PendingNote | undefined>(NOTES_STORE, 'readonly', (s) => s.get(key));
const putEntry = (e: PendingNote) => idb(NOTES_STORE, 'readwrite', (s) => s.put(e));
const allEntries = () => idb<PendingNote[]>(NOTES_STORE, 'readonly', (s) => s.getAll());

async function queue(e: PendingNote): Promise<void> {
  await putEntry(e);
  refreshCounts();
  scheduleFlush(1000);
}

// The service worker keeps the last notes list it saw for each book, to show
// when the server is down. After a write, fetch the list again (it goes through
// the worker, which stores it) so that copy includes the change. Debounced, as
// autosave writes often.
const refreshTimers = new Map<number, number>();
function refreshCachedList(bookId: number) {
  clearTimeout(refreshTimers.get(bookId));
  refreshTimers.set(bookId, window.setTimeout(() => {
    refreshTimers.delete(bookId);
    api.notes(bookId).catch(() => {});
  }, 2000));
}

/** Create a note. Offline, it gets a temporary negative id until it reaches the server. */
export async function createNote(bookId: number, n: NewNote): Promise<Note> {
  const clientId = newClientId();
  try {
    const created = await api.createNote(bookId, { ...n, client_id: clientId });
    refreshCachedList(bookId);
    return created;
  } catch (e) {
    if (!retryable(e) || userId === null) throw e;
  }
  const now = Date.now();
  const note: Note = {
    id: tempId(), book_id: bookId, page: n.page, body_md: n.body_md ?? '', quote: n.quote ?? null,
    anchor_start: n.anchor_start ?? null, anchor_end: n.anchor_end ?? null, color: n.color ?? null,
    created_at: now, updated_at: now, deleted_at: null, client_id: clientId,
  };
  await queue({ key: clientId, userId, bookId, serverId: null, create: true, fields: [], note, ts: now });
  return note;
}

/**
 * Change a note. Returns the note as it now is and whether the change is only
 * on this device so far. Throws only when the server refuses the change.
 */
export async function updateNote(note: Note, patch: NotePatch): Promise<{ note: Note; queued: boolean }> {
  const key = keyFor(note);
  const fields = Object.keys(patch) as Field[];
  let existing: PendingNote | undefined;
  try { existing = await getEntry(key); } catch { /* no IndexedDB: send directly */ }

  // Something for this note is already waiting: add to it so the order holds.
  if (existing && existing.userId === userId) {
    existing.note = apply({ ...existing.note, id: note.id < 0 ? existing.note.id : note.id }, patch);
    existing.fields = [...new Set([...existing.fields, ...fields])];
    existing.ts = Date.now();
    delete existing.failed;
    await queue(existing);
    return { note: existing.note, queued: true };
  }

  const serverId = serverIdOf(note);
  if (serverId !== null) {
    try {
      const updated = await api.updateNote(serverId, patch);
      refreshCachedList(note.book_id);
      return { note: updated, queued: false };
    } catch (e) {
      if (!retryable(e) || userId === null) throw e;
    }
  }
  // Server unreachable, or a note made offline that this tab doesn't know the
  // server id of (another tab sent it): the create is idempotent, so resend it.
  if (userId === null) throw new ApiError(0, 'Not signed in');
  const updated = apply(note, patch);
  await queue({
    key, userId, bookId: note.book_id, serverId, create: serverId === null, fields,
    note: { ...updated, id: serverId ?? note.id }, ts: Date.now(),
  });
  return { note: updated, queued: true };
}

/** A book's notes from the server with this device's unsent changes laid over them. */
export async function withPending(bookId: number, notes: Note[]): Promise<Note[]> {
  let entries: PendingNote[] = [];
  try { entries = (await allEntries()).filter((e) => e.bookId === bookId && e.userId === userId); } catch {}
  if (!entries.length) return notes;
  const out = [...notes];
  for (const e of entries) {
    const i = out.findIndex((n) => (e.note.client_id && n.client_id === e.note.client_id) || (e.serverId !== null && n.id === e.serverId));
    if (e.note.deleted_at) { if (i >= 0) out.splice(i, 1); continue; }
    if (i >= 0) out[i] = e.note; else out.push(e.note);
  }
  return sortNotes(out);
}

export const sortNotes = (l: Note[]) => l.sort((a, b) => a.page - b.page || (a.anchor_start ?? 0) - (b.anchor_start ?? 0) || a.created_at - b.created_at);

// Sending.

let flushing = false;
let again = false;
let timer: number | undefined;
let delay = 2000;

function scheduleFlush(ms: number) {
  clearTimeout(timer);
  timer = window.setTimeout(flushNotes, ms);
}

/** Send this user's queue. Retries with backoff while the server is unreachable. */
export async function flushNotes(): Promise<void> {
  if (userId === null) return;
  if (flushing) { again = true; return; }
  flushing = true;
  clearTimeout(timer);
  let retry = false;
  try {
    // One tab at a time, so two tabs don't send the same entry together.
    const run = () => flushAll().then((r) => { retry = r; });
    if (navigator.locks) await navigator.locks.request('bookman-notes-sync', run);
    else await run();
  } finally {
    flushing = false;
    refreshCounts();
  }
  if (again) { again = false; return flushNotes(); }
  if (retry) { scheduleFlush(delay); delay = Math.min(delay * 2, 60000); } else delay = 2000;
}

/** Returns true if something is left to retry. */
async function flushAll(): Promise<boolean> {
  let entries: PendingNote[];
  try { entries = await allEntries(); } catch { return false; }
  entries = entries.filter((e) => e.userId === userId && !e.failed).sort((a, b) => a.ts - b.ts);
  for (const e of entries) {
    try {
      await send(e);
    } catch (err) {
      if (retryable(err)) return true; // the server is still away (or we were signed out): stop here
      const cur = await getEntry(e.key);
      if (cur && cur.ts === e.ts) await putEntry({ ...cur, failed: (err as Error).message || 'Refused by the server' });
    }
  }
  return false;
}

async function send(e: PendingNote) {
  const want = e.note;
  let id = e.serverId;
  let server: Note | null = null;
  const patch: NotePatch = {};
  if (e.create) {
    server = await api.createNote(e.bookId, {
      page: want.page, body_md: want.body_md, quote: want.quote, anchor_start: want.anchor_start,
      anchor_end: want.anchor_end, color: want.color, client_id: want.client_id ?? e.key,
    });
    id = server.id;
    // A retried create returns the note as first sent; bring it up to date.
    if (server.body_md !== want.body_md) patch.body_md = want.body_md;
    if ((server.color ?? null) !== (want.color ?? null)) patch.color = want.color ?? '';
    if (!!server.deleted_at !== !!want.deleted_at) patch.deleted = !!want.deleted_at;
  } else {
    for (const f of e.fields) {
      if (f === 'body_md') patch.body_md = want.body_md;
      if (f === 'color') patch.color = want.color ?? '';
      if (f === 'deleted') patch.deleted = !!want.deleted_at;
    }
  }
  if (id === null) return;
  if (Object.keys(patch).length) server = await api.updateNote(id, patch);
  if (want.id < 0) resolved.set(want.id, id);

  // Done, unless the note was changed again while this was being sent.
  const cur = await getEntry(e.key);
  let shown: Note;
  if (cur && cur.ts === e.ts) {
    await idb(NOTES_STORE, 'readwrite', (s) => s.delete(e.key));
    shown = server ?? { ...want, id };
  } else if (cur) {
    await putEntry({ ...cur, create: false, serverId: id, note: { ...cur.note, id } });
    again = true;
    shown = { ...cur.note, id };
  } else return;
  refreshCachedList(e.bookId);
  for (const f of listeners) f(e.bookId, want.id, shown);
}

async function refreshCounts() {
  try {
    const mine = (await allEntries()).filter((e) => e.userId === userId);
    notesSync.pending = mine.filter((e) => !e.failed).length;
    notesSync.failed = mine.length - notesSync.pending;
  } catch {
    notesSync.pending = notesSync.failed = 0;
  }
}

// For the Settings page.

export async function pendingNotes(): Promise<PendingNote[]> {
  try { return (await allEntries()).filter((e) => e.userId === userId).sort((a, b) => a.ts - b.ts); } catch { return []; }
}

export async function retryNote(key: string) {
  const e = await getEntry(key);
  if (e) { delete e.failed; await putEntry(e); }
  refreshCounts();
  await flushNotes();
}

export async function discardNote(key: string) {
  await idb(NOTES_STORE, 'readwrite', (s) => s.delete(key));
  refreshCounts();
}
