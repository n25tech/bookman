<script lang="ts">
  import { createNote, sortNotes, updateNote } from '../lib/notes.svelte';
  import type { Book, Note } from '../lib/types';
  import NoteCard from './NoteCard.svelte';

  let { book, notes = $bindable(), currentPage, focusId = $bindable(), onjump, onpage, onclose }: {
    book: Book; notes: Note[]; currentPage: number; focusId: number | null; onjump: (n: Note) => void;
    onpage: (page: number) => void; onclose: () => void;
  } = $props();

  let tab = $state<'page' | 'all'>('page');
  let undo = $state<{ note: Note; timer: number } | null>(null);
  let error = $state('');

  // Book notes (page 0) are pinned above the page notes on every page.
  const bookNotes = $derived(notes.filter((n) => n.page === 0));
  const visible = $derived(tab === 'page' ? notes.filter((n) => n.page === currentPage) : notes.filter((n) => n.page > 0));

  async function addPageNote(page = currentPage) {
    error = '';
    try {
      const n = await createNote(book.id, { page, body_md: '' });
      notes = sortNotes([...notes, n]);
      focusId = n.id;
    } catch (e: any) { error = e.message; }
  }

  async function remove(n: Note) {
    try {
      await updateNote(n, { deleted: true });
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
      // The note may have reached the server since it was deleted: use its current id.
      const cur = { ...n, id: n.id < 0 ? notes.find((x) => x.client_id === n.client_id)?.id ?? n.id : n.id };
      const r = await updateNote(cur, { deleted: false });
      notes = sortNotes([...notes.filter((x) => !(r.note.client_id && x.client_id === r.note.client_id)), r.note]);
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
    <section class="booknotes">
      <div class="row">
        <h3>Book notes</h3>
        <span class="spacer"></span>
        <button class="ghost small" onclick={() => addPageNote(0)} title="A note about the whole book, shown on every page">+ Book note</button>
      </div>
      {#each bookNotes as n (n.client_id || n.id)}
        <NoteCard note={n} autofocus={focusId === n.id} {currentPage} {onjump} {onpage} ondelete={remove} onchange={changed} />
      {:else}
        <p class="faint hint">Thoughts on the whole book, kept on every page. Link to pages with [[p. 12]] or the 🔗 button.</p>
      {/each}
    </section>
    <button class="add" onclick={() => addPageNote()}>+ Note on page {currentPage}</button>
    {#if book.format === 'pdf' && !notes.length}
      <p class="faint hint">Tip: select text in the book to add a highlighted note.</p>
    {/if}
    {#if error}<p class="error">{error}</p>{/if}
    {#each visible as n (n.client_id || n.id)}
      <NoteCard note={n} autofocus={focusId === n.id} {currentPage} {onjump} {onpage} ondelete={remove} onchange={changed} />
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
  .booknotes { display: flex; flex-direction: column; gap: 8px; padding-bottom: 10px; border-bottom: 1px solid var(--border); }
  .booknotes h3 { margin: 0; font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-muted); }
  .booknotes .hint { text-align: left; margin: 0; }
  .hint, .empty { font-size: 0.88rem; text-align: center; margin: 8px 0; }
  .toast { position: absolute; bottom: 60px; }
</style>
