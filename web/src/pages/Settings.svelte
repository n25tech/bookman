<script lang="ts">
  import { onMount } from 'svelte';
  import { bytes, dateTime } from '../lib/format';
  import {
    budget, clearPageCache, connection, isSaved, offline, removeAllBooks, removeBook, saveSettings, savedBooks, type SavedBook,
  } from '../lib/offline.svelte';
  import { discardNote, flushNotes, notesSync, pendingNotes, retryNote, type PendingNote } from '../lib/notes.svelte';

  // Settings here are for this browser on this device.
  const GB = 1024 ** 3;
  const sizes = [0.25, 0.5, 1, 2, 5, 10, 20].map((g) => g * GB);

  let usage = $state<number | null>(null);
  let quota = $state<number | null>(null);
  let persisted = $state<boolean | null>(null);
  let limit = $state(0);
  let books = $state<SavedBook[]>([]);
  let notes = $state<PendingNote[]>([]);
  let notice = $state('');

  async function refresh() {
    const est = await navigator.storage?.estimate?.().catch(() => undefined);
    usage = est?.usage ?? null;
    quota = est?.quota ?? null;
    persisted = (await navigator.storage?.persisted?.().catch(() => null)) ?? null;
    limit = await budget().catch(() => offline.settings.maxBytes);
    const list = savedBooks().filter((b) => b.at > 0).sort((a, b) => b.at - a.at);
    const ok = await Promise.all(list.map((b) => isSaved(b.id)));
    books = list.filter((_, i) => ok[i]);
    notes = await pendingNotes();
  }
  onMount(() => {
    refresh();
    // Keep the numbers current while a book is being saved or notes are syncing.
    const t = setInterval(refresh, 3000);
    return () => clearInterval(t);
  });

  const savedTotal = $derived(books.reduce((s, b) => s + b.size, 0));

  function setAutoSave(v: boolean) { saveSettings({ ...offline.settings, autoSave: v }); }
  async function setMax(v: number) {
    if (v < savedTotal && !confirm(`Saved books use ${bytes(savedTotal)}. Lowering the space to ${bytes(v)} removes the least recently opened ones. Continue?`)) {
      refresh();
      return;
    }
    saveSettings({ ...offline.settings, maxBytes: v });
    setTimeout(refresh, 300);
  }
  async function persist() {
    const ok = await navigator.storage?.persist?.().catch(() => false);
    notice = ok ? 'Storage is now kept: the browser won’t clear saved books to free space.' : 'The browser declined. Installing the site or bookmarking it often helps.';
    refresh();
  }
  async function removeOne(b: SavedBook) { await removeBook(b.id); refresh(); }
  async function removeAll() {
    if (!confirm('Remove every book saved on this device? Reading progress and notes are not affected.')) return;
    await removeAllBooks();
    notice = 'Removed all saved books.';
    refresh();
  }
  async function clearPages() {
    await clearPageCache();
    notice = 'Cleared the cached pages and covers. Saved books are kept.';
    refresh();
  }

  async function syncNow() { await flushNotes(); refresh(); }
  async function retry(n: PendingNote) { await retryNote(n.key); refresh(); }
  async function copy(n: PendingNote) {
    try { await navigator.clipboard.writeText(n.note.body_md); notice = 'Note text copied.'; } catch { notice = 'Copy failed.'; }
  }
  async function discard(n: PendingNote) {
    if (!confirm('Discard this change? The text below will be lost.\n\n' + n.note.body_md.slice(0, 300))) return;
    await discardNote(n.key);
    refresh();
  }
  const excerpt = (s: string) => (s.trim() ? (s.length > 140 ? s.slice(0, 140) + '…' : s) : '(empty)');
</script>

<div class="page settings">
  <h1>Settings</h1>
  <p class="muted">These settings are for this browser on this device.</p>
  {#if notice}<p class="notice">{notice}</p>{/if}

  <section class="card">
    <h2>Sync</h2>
    <p class="muted">
      Notes and reading progress written while the server is unreachable are kept on this device and sent when it is back.
      {#if connection.offline}<strong>The server can't be reached right now.</strong>{/if}
    </p>
    {#if notes.length === 0}
      <p>Everything is synced.</p>
    {:else}
      <div class="row">
        <span>{notesSync.pending} waiting{notesSync.failed ? `, ${notesSync.failed} refused by the server` : ''}</span>
        <span class="spacer"></span>
        <button onclick={syncNow}>Sync now</button>
      </div>
      <ul class="list">
        {#each notes as n (n.key)}
          <li>
            <div class="row">
              <a href="/book/{n.bookId}">{n.note.page === 0 ? 'Book note' : `Page ${n.note.page}`}</a>
              <span class="faint small">{n.create ? 'new' : n.note.deleted_at ? 'deleted' : 'edited'} · {dateTime(n.ts)}</span>
              <span class="spacer"></span>
              {#if n.failed}
                <button class="small" onclick={() => retry(n)}>Retry</button>
                <button class="small" onclick={() => copy(n)}>Copy text</button>
                <button class="small danger" onclick={() => discard(n)}>Discard</button>
              {/if}
            </div>
            <p class="excerpt">{excerpt(n.note.body_md)}</p>
            {#if n.failed}<p class="error small">Not synced: {n.failed}</p>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h2>Offline reading</h2>
    {#if !offline.supported}
      <p class="muted">This browser can't keep books offline (it needs a secure https connection and service workers).</p>
    {:else}
      <label class="check">
        <input type="checkbox" checked={offline.settings.autoSave} onchange={(e) => setAutoSave(e.currentTarget.checked)} />
        Save each book I open, so it can be read with the server down
      </label>
      <p class="faint small">Books can also be saved one at a time from their page. Saving pauses while the browser's data saver is on.</p>

      <label class="field">
        Space for saved books
        <select value={offline.settings.maxBytes} onchange={(e) => setMax(Number(e.currentTarget.value))}>
          {#each sizes as s}<option value={s}>{bytes(s)}</option>{/each}
        </select>
      </label>
      {#if limit < offline.settings.maxBytes}
        <p class="faint small">The browser lets this site use {quota ? bytes(quota) : 'less'}, so at most {bytes(limit)} is used.</p>
      {/if}
      <p class="faint small">When the space is full, the least recently opened books are removed first.</p>

      <div class="usage">
        <div class="bar"><div style="width: {Math.min(100, (savedTotal / Math.max(1, limit)) * 100)}%"></div></div>
        <span class="small">{bytes(savedTotal)} of {bytes(limit)} used by {books.length} saved {books.length === 1 ? 'book' : 'books'}</span>
        {#if usage !== null && quota}
          <span class="faint small">This site uses {bytes(usage)} in total; the browser allows {bytes(quota)}.</span>
        {/if}
        {#if persisted === false}
          <span class="faint small">The browser may clear saved books when the device is low on space. <button class="link" onclick={persist}>Ask to keep them</button></span>
        {/if}
      </div>

      {#if offline.saving}
        <p>Saving “{offline.saving.title}”… {Math.round((offline.saving.done / Math.max(1, offline.saving.total)) * 100)}%</p>
      {/if}
      {#if offline.lastError}<p class="error small">{offline.lastError}</p>{/if}

      {#if books.length}
        <ul class="list">
          {#each books as b (b.id)}
            <li class="row">
              <a href="/book/{b.id}">{b.title}</a>
              <span class="faint small">{bytes(b.size)} · opened {dateTime(b.at)}</span>
              <span class="spacer"></span>
              <button class="small" onclick={() => removeOne(b)}>Remove</button>
            </li>
          {/each}
        </ul>
      {/if}
      <div class="row actions">
        <button onclick={removeAll} disabled={!books.length && !offline.saving}>Remove all saved books</button>
        <button onclick={clearPages} title="The copies of library pages, book details and covers kept for when the server is down">Clear cached pages</button>
      </div>
    {/if}
  </section>
</div>

<style>
  .settings { max-width: 760px; display: flex; flex-direction: column; gap: 16px; }
  h1 { margin: 0; }
  h2 { margin: 0 0 8px; font-size: 1.1rem; }
  section { display: flex; flex-direction: column; gap: 8px; }
  p { margin: 0; }
  .small { font-size: 0.85rem; }
  .notice { padding: 8px 12px; border-radius: 6px; background: var(--accent-soft); }
  .check { display: flex; gap: 8px; align-items: center; }
  .field { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .usage { display: flex; flex-direction: column; gap: 4px; margin: 4px 0; }
  .bar { height: 8px; border-radius: 4px; background: var(--bg-sunken); overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
  .list li { padding: 8px 0; border-top: 1px solid var(--border); min-width: 0; }
  .list a { overflow-wrap: anywhere; }
  .excerpt { font-size: 0.88rem; color: var(--fg-muted); white-space: pre-wrap; overflow-wrap: anywhere; margin-top: 4px; }
  .actions { margin-top: 4px; }
  .danger { color: var(--danger); }
  .link { border: 0; background: none; padding: 0; color: var(--accent); text-decoration: underline; cursor: pointer; font: inherit; }
</style>
