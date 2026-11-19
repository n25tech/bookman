<script lang="ts">
  import { api } from '../lib/api';
  import { humanize } from '../lib/format';
  import type { AdminLibrary } from '../lib/types';
  import FolderPicker from './FolderPicker.svelte';

  let { library = null, onsaved, oncancel }: { library?: AdminLibrary | null; onsaved: (l: AdminLibrary) => void; oncancel: () => void } = $props();

  let name = $state(library?.name ?? '');
  let path = $state(library?.path ?? '');
  let exclude = $state((library?.exclude ?? []).join('\n'));
  let picking = $state(!library);
  let error = $state('');
  let busy = $state(false);

  function picked(p: string) {
    path = p;
    picking = false;
    if (!name.trim()) name = humanize(p.split('/').filter(Boolean).pop() ?? p);
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    busy = true;
    const body = { name: name.trim(), path, exclude: exclude.split('\n').map((x) => x.trim()).filter(Boolean) };
    try {
      onsaved(library ? await api.updateLibrary(library.id, body) : await api.createLibrary(body));
    } catch (err: any) { error = err.message; }
    busy = false;
  }
</script>

<form class="card editor" onsubmit={save}>
  <h3>{library ? `Edit ${library.name}` : 'Add a library'}</h3>
  <label>Folder
    <div class="row">
      <input value={path} readonly placeholder="Choose a folder…" onclick={() => (picking = true)} />
      <button type="button" onclick={() => (picking = !picking)}>{picking ? 'Close' : 'Browse…'}</button>
    </div>
  </label>
  {#if picking}
    <FolderPicker start={path} onpick={picked} oncancel={() => (picking = false)} />
  {/if}
  <label>Name <input bind:value={name} required maxlength="100" placeholder="e.g. Books" /></label>
  <label>Skip these sub-folders <span class="faint">(optional, one per line, relative to the folder)</span>
    <textarea bind:value={exclude} rows="3" placeholder="comics"></textarea>
  </label>
  <p class="faint small">
    A sub-folder that is its own library is left to that library automatically. Books are grouped by their folders,
    and the library is scanned as soon as you save.
  </p>
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  <div class="row">
    <button type="submit" class="primary" disabled={busy || !path || !name.trim()}>{library ? 'Save' : 'Add library'}</button>
    <button type="button" onclick={oncancel}>Cancel</button>
  </div>
</form>

<style>
  .editor { display: flex; flex-direction: column; gap: 12px; }
  h3 { margin: 0; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 0.9rem; color: var(--fg-muted); }
  label .row input { flex: 1; min-width: 0; cursor: pointer; }
  textarea { resize: vertical; font-family: ui-monospace, monospace; font-size: 0.85rem; }
  .small { font-size: 0.82rem; margin: 0; }
</style>
