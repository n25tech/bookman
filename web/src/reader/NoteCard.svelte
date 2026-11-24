<script lang="ts">
  import { updateNote } from '../lib/notes.svelte';
  import { renderMarkdown } from '../lib/markdown';
  import { ago } from '../lib/format';
  import type { Note } from '../lib/types';

  let { note, autofocus = false, currentPage, onjump, onpage, ondelete, onchange }: {
    note: Note; autofocus?: boolean; currentPage: number; onjump: (n: Note) => void; onpage: (page: number) => void;
    ondelete: (n: Note) => void; onchange: (n: Note) => void;
  } = $props();
  let textarea = $state<HTMLTextAreaElement>();

  // Insert a link to the page being read at the cursor, e.g. to continue a
  // book note from another page.
  function insertPageLink() {
    const link = `[[p. ${currentPage}]]`;
    const el = textarea;
    const start = el?.selectionStart ?? body.length;
    const end = el?.selectionEnd ?? body.length;
    const before = body.slice(0, start);
    const sep = before && !/\s$/.test(before) ? ' ' : '';
    body = before + sep + link + body.slice(end);
    onInput();
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      const pos = start + sep.length + link.length;
      el.setSelectionRange(pos, pos);
    });
  }

  function onPreviewClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest<HTMLAnchorElement>('a[data-page]');
    if (!a) return;
    e.preventDefault();
    onpage(Number(a.dataset.page));
  }

  let editing = $state(autofocus || !note.body_md);
  let body = $state(note.body_md);
  let saving = $state<'idle' | 'saving' | 'saved' | 'queued' | 'error'>('idle');
  let saveError = $state('');
  let timer: number | undefined;
  const colors = ['yellow', 'green', 'blue', 'pink'];

  async function save() {
    clearTimeout(timer);
    timer = undefined;
    if (body === note.body_md) return;
    saving = 'saving';
    try {
      // Kept on this device and sent later if the server can't be reached.
      const r = await updateNote(note, { body_md: body });
      onchange(r.note);
      saving = r.queued ? 'queued' : 'saved';
    } catch (e: any) {
      saving = 'error';
      saveError = e?.message ?? 'refused by the server';
    }
  }
  // Autosave 800 ms after typing stops.
  function onInput() { clearTimeout(timer); saving = 'idle'; timer = window.setTimeout(save, 800); }

  async function setColor(c: string) {
    try { onchange((await updateNote(note, { color: c })).note); } catch {}
  }

  function focusOnMount(el: HTMLTextAreaElement) { if (autofocus) setTimeout(() => el.focus(), 0); }

  $effect(() => () => { if (timer !== undefined) save(); });
</script>

<article class="note" style="--c: var(--hl-{note.color || 'yellow'})">
  <header>
    {#if note.page === 0}
      <span class="pg book" title="A note about the whole book">Book</span>
    {:else}
      <button class="ghost pg" onclick={() => onjump(note)} title="Go to page {note.page}">p. {note.page}</button>
    {/if}
    <span class="faint small">{ago(note.updated_at)}</span>
    <span class="spacer"></span>
    {#if saving === 'saving'}<span class="faint small">Saving…</span>{:else if saving === 'saved'}<span class="faint small">Saved</span>{:else if saving === 'queued'}<span class="faint small" title="The server can't be reached. The note is kept on this device and sent when it is back.">Saved on this device</span>{:else if saving === 'error'}<span class="error small" title={saveError}>Not saved: {saveError}</span>{/if}
    <button class="ghost icon small" onclick={() => { if (editing) save(); editing = !editing; }} title={editing ? 'Preview' : 'Edit'}>{editing ? 'Preview' : 'Edit'}</button>
    <button class="ghost icon small danger" onclick={() => ondelete(note)} title="Delete note" aria-label="Delete note">✕</button>
  </header>
  {#if note.quote}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
    <blockquote onclick={() => onjump(note)}>{note.quote}</blockquote>
  {/if}
  {#if editing}
    <textarea bind:this={textarea} bind:value={body} oninput={onInput} onblur={save} rows={note.page === 0 ? 6 : 4}
      placeholder={note.page === 0 ? 'Notes on the whole book (Markdown). Link pages with [[p. 12]].' : 'Write a note (Markdown)…'} use:focusOnMount></textarea>
    <div class="tools">
      <button class="ghost small linkbtn" onmousedown={(e) => e.preventDefault()} onclick={insertPageLink} title="Insert a link to the page you're reading">🔗 p. {currentPage}</button>
      <span class="faint small">[[p. 12]] links a page</span>
    </div>
    {#if note.quote}
      <div class="colors">
        {#each colors as c}
          <button class="swatch" class:on={(note.color || 'yellow') === c} style="background: var(--hl-{c})" onclick={() => setColor(c)} aria-label="Highlight {c}"></button>
        {/each}
      </div>
    {/if}
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="md" ondblclick={() => (editing = true)} onclick={onPreviewClick}>
      {#if body.trim()}{@html renderMarkdown(body)}{:else}<span class="faint">Empty note. Double-click to edit.</span>{/if}
    </div>
  {/if}
</article>

<style>
  .note { border: 1px solid var(--border); border-left: 4px solid var(--c); border-radius: 6px; padding: 8px 10px; background: var(--bg-elev); display: flex; flex-direction: column; gap: 6px; }
  header { display: flex; align-items: center; gap: 6px; }
  .pg { padding: 1px 6px; font-weight: 600; font-size: 0.82rem; }
  .pg.book { border-radius: 4px; background: var(--accent-soft); color: var(--fg); }
  .tools { display: flex; align-items: center; gap: 8px; }
  .linkbtn { padding: 2px 8px; }
  .md :global(a.pagelink) { padding: 0 4px; border-radius: 4px; background: var(--accent-soft); white-space: nowrap; }
  .small { font-size: 0.78rem; }
  .spacer { flex: 1; }
  blockquote { margin: 0; padding: 4px 8px; background: var(--c); border-radius: 4px; font-size: 0.88rem; cursor: pointer; max-height: 8em; overflow: auto; }
  textarea { width: 100%; resize: vertical; font-size: 0.9rem; min-height: 70px; }
  .md { font-size: 0.9rem; overflow-wrap: anywhere; }
  .md :global(p) { margin: 0 0 0.5em; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(blockquote) { border-left: 3px solid var(--border); margin: 0 0 0.5em; padding-left: 8px; color: var(--fg-muted); }
  .md :global(pre) { background: var(--bg-sunken); padding: 6px; border-radius: 4px; overflow: auto; }
  .md :global(ul), .md :global(ol) { margin: 0 0 0.5em; padding-left: 1.3em; }
  .colors { display: flex; gap: 6px; }
  .swatch { width: 20px; height: 20px; padding: 0; border-radius: 50%; border: 2px solid transparent; }
  .swatch.on { border-color: var(--fg); }
</style>
