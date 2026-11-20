<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { bytes, dateTime, duration, percent, ago, humanize } from '../lib/format';
  import { deviceLabel } from '../lib/device';
  import type { Book, Library, ReadingSession } from '../lib/types';
  import { session } from '../lib/session.svelte';
  import Cover from '../components/Cover.svelte';

  let { id }: { id: number } = $props();
  let book = $state<Book | null>(null);
  let library = $state<Library | null>(null);
  let noteCount = $state(0);
  let sessions = $state<ReadingSession[]>([]);
  let error = $state('');
  let busy = $state(false);
  let editing = $state(false);
  let editTitle = $state('');
  let editAuthor = $state('');
  let editError = $state('');

  async function load() {
    try {
      const r = await api.book(id);
      book = r.book; library = r.library; noteCount = r.note_count;
      sessions = await api.sessions(id);
    } catch (e: any) { error = e.message; }
  }
  onMount(load);

  async function toggleFinished() {
    if (!book) return;
    busy = true;
    try { book.progress = await api.setFinished(id, !book.progress?.finished); } catch (e: any) { alert(e.message); }
    busy = false;
  }
  async function reset() {
    if (!book || !confirm('Reset reading progress for this book?')) return;
    busy = true;
    try {
      await api.resetProgress(id);
      book.progress = null;
      try { localStorage.removeItem(`bookman:pos:${id}`); } catch {}
    } catch (e: any) { alert(e.message); }
    busy = false;
  }
  function startEdit() {
    if (!book) return;
    editTitle = book.title; editAuthor = book.author; editError = ''; editing = true;
  }
  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!book) return;
    busy = true; editError = '';
    try {
      const r = await api.updateBook(id, { title: editTitle.trim(), author: editAuthor.trim() });
      book.title = r.book.title; book.author = r.book.author;
      editing = false;
    } catch (err: any) { editError = err.message; }
    busy = false;
  }
  // A readable title from the file name, as a starting point for messy titles.
  function fromFilename() {
    const base = (book?.rel_path ?? '').split('/').pop() ?? '';
    editTitle = humanize(base.replace(/\.[^.]+$/, '').replace(/[_-](compress(ed)?|scan(ned)?|ocr)$/i, '').replace(/-+/g, ' '));
  }

  const p = $derived(book?.progress);
  // The folders the file is in, for the breadcrumbs.
  const folders = $derived.by(() => {
    const parts = (book?.rel_path ?? '').split('/').slice(0, -1);
    return parts.map((name, i) => ({ name, path: parts.slice(0, i + 1).join('/') }));
  });
</script>

<div class="page">
  {#if error}
    <div class="empty"><h2>Couldn't load the book</h2><p>{error}</p></div>
  {:else if !book}
    <div class="empty">Loading…</div>
  {:else}
    <div class="layout">
      <div class="cover"><Cover bookId={book.id} etag={book.cover_etag} title={book.title} format={book.format} /></div>
      <div class="info">
        <p class="crumbs">
          <a href="/library/{book.library_id}">{library?.name}</a>
          {#each folders as f (f.path)}
            / <a href="/library/{book.library_id}?path={encodeURIComponent(f.path)}">{humanize(f.name)}</a>
          {/each}
        </p>
        {#if editing}
          <form class="card editor" onsubmit={saveEdit}>
            <label>Title <input bind:value={editTitle} required maxlength="500" /></label>
            <label>Author <span class="faint">(optional)</span> <input bind:value={editAuthor} maxlength="500" /></label>
            <p class="faint small">Saved in bookman only; the file isn't changed. Rescans keep your edits.</p>
            {#if editError}<p class="error" role="alert">{editError}</p>{/if}
            <div class="row">
              <button type="submit" class="primary" disabled={busy || !editTitle.trim()}>Save</button>
              <button type="button" onclick={() => (editing = false)}>Cancel</button>
              <button type="button" class="ghost" onclick={fromFilename} title="Rebuild the title from the file name">Use file name</button>
            </div>
          </form>
        {:else}
          <div class="title-row">
            <h1>{book.title}</h1>
            {#if session.user?.is_admin}
              <button class="ghost edit" onclick={startEdit} title="Edit title and author">Edit</button>
            {/if}
          </div>
          {#if book.author}<p class="author">{book.author}</p>{/if}
        {/if}

        {#if book.status === 'missing'}
          <p class="error">This file is missing from disk. Its progress and notes are kept until it comes back.</p>
        {:else if book.status === 'error'}
          <p class="error">This file couldn't be read: {book.error_msg}</p>
        {/if}

        <div class="actions row">
          <a class="btn primary" href="/read/{book.id}" class:disabled={book.status !== 'ok'}>
            {p && !p.finished ? `Continue · page ${p.page}` : p?.finished ? 'Read again' : 'Read'}
          </a>
          <button onclick={toggleFinished} disabled={busy}>{p?.finished ? 'Mark unfinished' : 'Mark finished'}</button>
          <button onclick={reset} disabled={busy || !p}>Reset progress</button>
        </div>

        <div class="progress">
          <div class="bar"><div style="width: {percent(p?.finished ? 1 : p?.percent)}"></div></div>
          <span class="muted">
            {#if p?.finished}Finished{:else if p}{percent(p.percent)} · page {p.page} of {book.page_count}{:else}Not started{/if}
            {#if p?.device} · last read {ago(p.updated_at)} on {deviceLabel(p.device)}{/if}
          </span>
        </div>

        <dl>
          <dt>Format</dt><dd>{book.format.toUpperCase()}</dd>
          <dt>Pages</dt><dd>{book.page_count}</dd>
          <dt>Size</dt><dd>{bytes(book.file_size)}</dd>
          <dt>Notes</dt><dd>{noteCount}{#if noteCount} · <a href="/api/books/{book.id}/notes/export.md" download>Export Markdown</a>{/if}</dd>
          <dt>Added</dt><dd>{dateTime(book.added_at)}</dd>
          <dt>File</dt><dd class="path">{book.rel_path}</dd>
        </dl>

        <h2>Reading history</h2>
        {#if sessions.length === 0}
          <p class="faint">No reading sessions yet.</p>
        {:else}
          <table>
            <thead><tr><th>When</th><th>Pages</th><th>Duration</th><th>Device</th></tr></thead>
            <tbody>
              {#each sessions as s (s.id)}
                <tr>
                  <td>{dateTime(s.started_at)}</td>
                  <td>{s.start_page === s.end_page ? s.start_page : `${s.start_page} → ${s.end_page}`}</td>
                  <td>{duration(s.ended_at - s.started_at)}</td>
                  <td>{deviceLabel(s.device)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .layout { display: grid; grid-template-columns: 260px 1fr; gap: 32px; align-items: start; }
  @media (max-width: 700px) { .layout { grid-template-columns: 1fr; } .cover { max-width: 200px; } }
  .crumbs { margin: 0 0 6px; font-size: 0.9rem; color: var(--fg-muted); }
  .title-row { display: flex; align-items: baseline; gap: 8px; }
  .title-row .edit { font-size: 0.85rem; color: var(--fg-muted); flex: none; }
  .editor { display: flex; flex-direction: column; gap: 12px; max-width: 560px; margin: 8px 0 12px; }
  .editor label { display: flex; flex-direction: column; gap: 4px; font-size: 0.9rem; color: var(--fg-muted); }
  .small { font-size: 0.82rem; margin: 0; }
  .author { margin-top: -4px; color: var(--fg-muted); font-size: 1.05rem; }
  .actions { margin: 18px 0; }
  .btn.disabled { pointer-events: none; opacity: 0.5; }
  .progress { display: flex; flex-direction: column; gap: 6px; margin-bottom: 20px; }
  .bar { height: 6px; background: var(--bg-sunken); border-radius: 3px; overflow: hidden; max-width: 420px; }
  .bar > div { height: 100%; background: var(--accent); }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 6px 18px; margin: 0 0 24px; }
  dt { color: var(--fg-faint); }
  dd { margin: 0; }
  .path { word-break: break-all; color: var(--fg-muted); font-size: 0.9rem; }
  table { border-collapse: collapse; width: 100%; max-width: 640px; font-size: 0.9rem; }
  th, td { text-align: left; padding: 6px 10px 6px 0; border-bottom: 1px solid var(--border); }
  th { color: var(--fg-faint); font-weight: 500; }
</style>
