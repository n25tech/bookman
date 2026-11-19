<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { router } from '../lib/router.svelte';
  import { deviceId, deviceLabel } from '../lib/device';
  import { percent } from '../lib/format';
  import { ProgressSync, localPosition, pendingFor, type Position } from '../lib/progress';
  import type { Book, Note, Progress } from '../lib/types';
  import ThemeToggle from '../components/ThemeToggle.svelte';
  import PdfViewer from './PdfViewer.svelte';
  import ComicViewer from './ComicViewer.svelte';
  import NotesPanel from './NotesPanel.svelte';
  import type { NewAnchor, ViewerApi } from './types';

  let { id }: { id: number } = $props();

  const PANEL_KEY = 'bookman:notes-width';
  const OPEN_KEY = 'bookman:notes-open';
  function stored(k: string, def: string) { try { return localStorage.getItem(k) ?? def; } catch { return def; } }

  let book = $state<Book | null>(null);
  let notes = $state<Note[]>([]);
  let start = $state<Position | null>(null);
  let pos = $state<Position>({ page: 1, offset: 0 });
  let error = $state('');
  let resume = $state<Progress | null>(null);
  let panelOpen = $state(stored(OPEN_KEY, matchMedia('(min-width: 900px)').matches ? '1' : '0') === '1');
  let panelWidth = $state(Number(stored(PANEL_KEY, '360')) || 360);
  let focusNote = $state<number | null>(null);
  let finished = $state(false);
  let viewer = $state<ViewerApi>();
  let sync: ProgressSync | null = null;

  const ahead = (a: Position, b: Position) => a.page - 1 + a.offset > b.page - 1 + b.offset + 0.001;

  onMount(() => {
    (async () => {
      try {
        const [r, n, pending] = await Promise.all([api.book(id), api.notes(id), pendingFor(id)]);
        if (r.book.status !== 'ok') { error = r.book.status === 'missing' ? 'This file is missing from disk.' : `This file can't be read: ${r.book.error_msg}`; return; }
        book = r.book;
        notes = n;
        const server = r.book.progress;
        finished = !!server?.finished;
        const local = localPosition(id);
        const me = deviceId();

        // Where to open: this device's own latest position unless the server
        // has something newer; if another device is further ahead, offer it.
        let s: Position = { page: 1, offset: 0 };
        if (server) s = { page: server.page, offset: server.offset };
        if (pending && (!server || pending.client_ts > server.client_ts)) s = { page: pending.page, offset: pending.offset };
        if (server && server.device && server.device !== me && local && ahead(server, local)) {
          s = { page: local.page, offset: local.offset };
          resume = server;
        } else if (!server && local) {
          s = { page: local.page, offset: local.offset };
        }
        s.page = Math.min(Math.max(1, s.page), r.book.page_count || 1);
        start = s;
        pos = s;

        sync = new ProgressSync(id);
        sync.prime(s);
        sync.onSaved = (p) => { finished = p.finished; };
      } catch (e: any) {
        error = e.message;
      }
    })();
    return () => sync?.destroy();
  });

  function onposition(p: Position) {
    pos = p;
    sync?.update(p);
    if (resume && !ahead(resume, p)) resume = null; // caught up by themselves
  }

  function jumpResume() {
    if (!resume) return;
    viewer?.goTo(resume.page, resume.offset);
    resume = null;
  }

  async function addAnchoredNote(a: NewAnchor) {
    try {
      const n = await api.createNote(id, { ...a, body_md: '', color: 'yellow' });
      notes = [...notes, n].sort((x, y) => x.page - y.page || (x.anchor_start ?? 0) - (y.anchor_start ?? 0));
      focusNote = n.id;
      setPanel(true);
    } catch (e: any) { alert(e.message); }
  }

  function setPanel(open: boolean) {
    panelOpen = open;
    try { localStorage.setItem(OPEN_KEY, open ? '1' : '0'); } catch {}
  }

  // Draggable divider; the width is remembered per device.
  function startDrag(e: PointerEvent) {
    e.preventDefault();
    const el = e.currentTarget as HTMLElement;
    el.setPointerCapture(e.pointerId);
    const startX = e.clientX, startW = panelWidth;
    const move = (ev: PointerEvent) => {
      panelWidth = Math.round(Math.min(Math.max(240, startW + (startX - ev.clientX)), window.innerWidth * 0.6));
    };
    const up = () => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
      try { localStorage.setItem(PANEL_KEY, String(panelWidth)); } catch {}
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', up);
  }

  function onKey(e: KeyboardEvent) {
    const t = e.target as HTMLElement;
    if (t.closest('input, textarea, select, [contenteditable]') || e.metaKey || e.altKey) return;
    if (e.ctrlKey) {
      if (e.key === 'f' && book?.format === 'pdf') { e.preventDefault(); (viewer as any)?.openSearch?.(); }
      return;
    }
    switch (e.key) {
      case 'ArrowRight': e.preventDefault(); viewer?.next(); break;
      case 'ArrowLeft': e.preventDefault(); viewer?.prev(); break;
      case 'ArrowDown':
        if (book?.format !== 'pdf') { e.preventDefault(); viewer?.pageDown(); }
        break;
      case 'ArrowUp':
        if (book?.format !== 'pdf') { e.preventDefault(); viewer?.pageUp(); }
        break;
      case 'PageDown': e.preventDefault(); viewer?.pageDown(); break;
      case 'PageUp': e.preventDefault(); viewer?.pageUp(); break;
      case ' ': e.preventDefault(); if (e.shiftKey) viewer?.pageUp(); else viewer?.pageDown(); break;
      case 'n': case 'N': e.preventDefault(); setPanel(!panelOpen); break;
      case 'Escape': exit(); break;
    }
  }

  function exit() {
    sync?.saveNow();
    router.go(book ? `/library/${book.library_id}` : '/');
  }

  function jumpToNote(n: Note) { viewer?.flashNote(n); }
  const shownPercent = $derived(book ? Math.min(1, (pos.page - 1 + pos.offset) / Math.max(1, book.page_count)) : 0);
</script>

<svelte:window onkeydown={onKey} />

<div class="reader" style="--panel-w: {panelWidth}px">
  <div class="top">
    <a class="btn ghost" href={book ? `/book/${book.id}` : '/'} title="Back to the book (Esc: library)">← Back</a>
    <div class="title" title={book?.title}>{book?.title ?? ''}</div>
    <span class="spacer"></span>
    <ThemeToggle />
    <button class="ghost" class:on={panelOpen} onclick={() => setPanel(!panelOpen)} title="Notes (n)">
      Notes{notes.length ? ` (${notes.length})` : ''}
    </button>
  </div>

  {#if error}
    <div class="empty"><h2>Can't open this book</h2><p>{error}</p><a href="/">Back to the library</a></div>
  {:else if book && start}
    <div class="body" class:with-panel={panelOpen}>
      <div class="viewer">
        {#if book.format === 'pdf'}
          <PdfViewer bind:this={viewer} {book} {start} {notes} {onposition} onaddnote={addAnchoredNote} onerror={(m) => (error = m)} />
        {:else}
          <ComicViewer bind:this={viewer} {book} {start} {onposition} />
        {/if}
      </div>
      {#if panelOpen}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="divider" onpointerdown={startDrag} title="Drag to resize"></div>
        <aside class="notes">
          <NotesPanel {book} bind:notes currentPage={pos.page} bind:focusId={focusNote} onjump={jumpToNote} onclose={() => setPanel(false)} />
        </aside>
      {/if}
    </div>
    <div class="bottom">
      <div class="bar" title="{percent(shownPercent)}"><div style="width: {percent(shownPercent)}"></div></div>
      <span class="small">Page {pos.page} of {book.page_count} · {percent(shownPercent)}{finished ? ' · finished' : ''}</span>
    </div>
    {#if resume}
      <div class="toast" role="status">
        You were on page {resume.page} on {deviceLabel(resume.device)}. Jump there?
        <button onclick={jumpResume}>Jump</button>
        <button onclick={() => (resume = null)} aria-label="Dismiss">✕</button>
      </div>
    {/if}
  {:else}
    <div class="empty">Opening…</div>
  {/if}
</div>

<style>
  .reader { height: 100vh; height: 100dvh; display: flex; flex-direction: column; background: var(--bg); }
  .top { display: flex; align-items: center; gap: 6px; padding: 4px 8px; border-bottom: 1px solid var(--border); background: var(--bg-elev); }
  .title { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .spacer { flex: 1; }
  .top button.on { background: var(--accent-soft); }
  .body { flex: 1; min-height: 0; display: flex; position: relative; }
  .viewer { flex: 1; min-width: 0; position: relative; }
  .divider { width: 6px; cursor: col-resize; background: var(--border); flex: none; touch-action: none; }
  .divider:hover { background: var(--accent); }
  .notes { width: var(--panel-w); flex: none; border-left: 1px solid var(--border); min-height: 0; position: relative; }
  .bottom { display: flex; align-items: center; gap: 10px; padding: 4px 12px; border-top: 1px solid var(--border); background: var(--bg-elev); }
  .bar { flex: 1; height: 4px; background: var(--bg-sunken); border-radius: 2px; overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); transition: width 0.2s; }
  .small { font-size: 0.8rem; color: var(--fg-muted); white-space: nowrap; }
  .toast { bottom: 44px; }

  /* Narrow screens: the notes panel becomes a bottom drawer over the viewer. */
  @media (max-width: 899px) {
    .divider { display: none; }
    .notes {
      position: absolute; left: 0; right: 0; bottom: 0; width: auto; height: 55%;
      border-left: 0; border-top: 1px solid var(--border); box-shadow: 0 -6px 20px rgb(0 0 0 / 0.2);
      border-radius: 12px 12px 0 0; overflow: hidden; z-index: 30;
    }
  }
</style>
