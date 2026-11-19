<script lang="ts">
  import { api } from '../lib/api';
  import type { Book, Note } from '../lib/types';
  import NoteCard from './NoteCard.svelte';

  let { book, notes = $bindable(), currentPage, focusId = $bindable(), onjump, onclose }: {
    book: Book; notes: Note[]; currentPage: number; focusId: number | null; onjump: (n: Note) => void; onclose: () => void;
  } = $props();

  let tab = $state<'page' | 'all'>('page');
  let undo = $state<{ note: Note; timer: number } | null>(null);
  let error = $state('');

  const visible = $derived(tab === 'page' ? notes.filter((n) => n.page === currentPage) : notes);

  async function addPageNote() {
    error = '';
    try {
      const n = await api.createNote(book.id, { page: currentPage, body_md: '' });
      notes = [...notes, n].sort((a, b) => a.page - b.page || (a.anchor_start ?? 0) - (b.anchor_start ?? 0));
      focusId = n.id;
    } catch (e: any) { error = e.message; }
  }

  async function remove(n: Note) {
    try {
      await api.deleteNote(n.id);
      notes = notes.filter((x) => x.id !== n.id);
      if (undo) clearTimeout(undo.timer);
      undo = { note: n, timer: window.setTimeout(() => (undo = null), 6000) };
    } catch (e: any) { error = e.message; }
  }

  async function restore() {
    if (!undo) return;
    const n = undo.note;
    clearTimeout(undo.timer);
    undo = null;
    try {
      const r = await api.updateNote(n.id, { deleted: false });
      notes = [...notes, r].sort((a, b) => a.page - b.page || (a.anchor_start ?? 0) - (b.anchor_start ?? 0));
    } catch (e: any) { error = e.message; }
  }

  function changed(n: Note) { notes = notes.map((x) => (x.id === n.id ? n : x)); }
</script>

<div class="panel">
  <div class="head">
    <div class="tabs" role="tablist">
      <button role="tab" class:on={tab === 'page'} aria-selected={tab === 'page'} onclick={() => (tab = 'page')}>This page</button>
      <button role="tab" class:on={tab === 'all'} aria-selected={tab === 'all'} onclick={() => (tab = 'all')}>All ({notes.length})</button>
    </div>
    <span class="spacer"></span>
    <a class="btn ghost small" href="/api/books/{book.id}/notes/export.md" download title="Export all notes as Markdown">Export</a>
    <button class="ghost icon" onclick={onclose} title="Hide notes (n)" aria-label="Hide notes">✕</button>
  </div>
  <div class="list">
    <button class="add" onclick={addPageNote}>+ Note on page {currentPage}</button>
    {#if book.format === 'pdf' && !notes.length}
      <p class="faint hint">Tip: select text in the book to add a highlighted note.</p>
    {/if}
    {#if error}<p class="error">{error}</p>{/if}
    {#each visible as n (n.id)}
      <NoteCard note={n} autofocus={focusId === n.id} {onjump} ondelete={remove} onchange={changed} />
    {:else}
      <p class="faint empty">{tab === 'page' ? 'No notes on this page.' : 'No notes in this book yet.'}</p>
    {/each}
  </div>
  {#if undo}
    <div class="toast">Note deleted <button onclick={restore}>Undo</button></div>
  {/if}
</div>

<style>
  .panel { height: 100%; display: flex; flex-direction: column; background: var(--bg); min-width: 0; }
  .head { display: flex; align-items: center; gap: 4px; padding: 4px 8px; border-bottom: 1px solid var(--border); background: var(--bg-elev); }
  .tabs { display: flex; gap: 2px; }
  .tabs button { border: 0; background: transparent; color: var(--fg-muted); padding: 5px 10px; border-radius: 6px; }
  .tabs button.on { background: var(--accent-soft); color: var(--fg); }
  .spacer { flex: 1; }
  .small { font-size: 0.85rem; padding: 4px 8px; }
  .list { flex: 1; overflow: auto; padding: 10px; display: flex; flex-direction: column; gap: 10px; }
  .add { justify-content: center; border-style: dashed; color: var(--fg-muted); }
  .hint, .empty { font-size: 0.88rem; text-align: center; margin: 8px 0; }
  .toast { position: absolute; bottom: 60px; }
</style>
