<script lang="ts">
  import { api } from '../lib/api';
  import { renderMarkdown } from '../lib/markdown';
  import { ago } from '../lib/format';
  import type { Note } from '../lib/types';

  let { note, autofocus = false, onjump, ondelete, onchange }: {
    note: Note; autofocus?: boolean; onjump: (n: Note) => void; ondelete: (n: Note) => void; onchange: (n: Note) => void;
  } = $props();

  let editing = $state(autofocus || !note.body_md);
  let body = $state(note.body_md);
  let saving = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let timer: number | undefined;
  const colors = ['yellow', 'green', 'blue', 'pink'];

  async function save() {
    clearTimeout(timer);
    timer = undefined;
    if (body === note.body_md) return;
    saving = 'saving';
    try {
      const n = await api.updateNote(note.id, { body_md: body });
      onchange(n);
      saving = 'saved';
    } catch {
      saving = 'error';
      timer = window.setTimeout(save, 5000);
    }
  }
  // Autosave 800 ms after typing stops.
  function onInput() { clearTimeout(timer); saving = 'idle'; timer = window.setTimeout(save, 800); }

  async function setColor(c: string) {
    try { onchange(await api.updateNote(note.id, { color: c })); } catch {}
  }

  function focusOnMount(el: HTMLTextAreaElement) { if (autofocus) setTimeout(() => el.focus(), 0); }

  $effect(() => () => { if (timer !== undefined) save(); });
</script>

<article class="note" style="--c: var(--hl-{note.color || 'yellow'})">
  <header>
    <button class="ghost pg" onclick={() => onjump(note)} title="Go to page {note.page}">p. {note.page}</button>
    <span class="faint small">{ago(note.updated_at)}</span>
    <span class="spacer"></span>
    {#if saving === 'saving'}<span class="faint small">Saving…</span>{:else if saving === 'saved'}<span class="faint small">Saved</span>{:else if saving === 'error'}<span class="error small">Not saved, retrying</span>{/if}
    <button class="ghost icon small" onclick={() => { if (editing) save(); editing = !editing; }} title={editing ? 'Preview' : 'Edit'}>{editing ? 'Preview' : 'Edit'}</button>
    <button class="ghost icon small danger" onclick={() => ondelete(note)} title="Delete note" aria-label="Delete note">✕</button>
  </header>
  {#if note.quote}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
    <blockquote onclick={() => onjump(note)}>{note.quote}</blockquote>
  {/if}
  {#if editing}
    <textarea bind:value={body} oninput={onInput} onblur={save} rows="4" placeholder="Write a note (Markdown)…" use:focusOnMount></textarea>
    {#if note.quote}
      <div class="colors">
        {#each colors as c}
          <button class="swatch" class:on={(note.color || 'yellow') === c} style="background: var(--hl-{c})" onclick={() => setColor(c)} aria-label="Highlight {c}"></button>
        {/each}
      </div>
    {/if}
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="md" ondblclick={() => (editing = true)}>
      {#if body.trim()}{@html renderMarkdown(body)}{:else}<span class="faint">Empty note. Double-click to edit.</span>{/if}
    </div>
  {/if}
</article>

<style>
  .note { border: 1px solid var(--border); border-left: 4px solid var(--c); border-radius: 6px; padding: 8px 10px; background: var(--bg-elev); display: flex; flex-direction: column; gap: 6px; }
  header { display: flex; align-items: center; gap: 6px; }
  .pg { padding: 1px 6px; font-weight: 600; font-size: 0.82rem; }
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
