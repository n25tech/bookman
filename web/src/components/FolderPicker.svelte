<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import type { FsListing } from '../lib/types';

  // Browse the server's folders (within media_roots) and pick one.
  let { start = '', onpick, oncancel }: { start?: string; onpick: (path: string) => void; oncancel: () => void } = $props();

  let listing = $state<FsListing | null>(null);
  let error = $state('');
  let loading = $state(false);

  async function open(path: string) {
    loading = true;
    error = '';
    try { listing = await api.fs(path); } catch (e: any) {
      error = e.message;
      if (!listing && path) { try { listing = await api.fs(''); } catch {} }
    }
    loading = false;
  }
  onMount(() => open(start));
</script>

<div class="picker">
  <div class="bar">
    <button type="button" class="ghost icon" disabled={listing?.parent == null || loading} onclick={() => open(listing!.parent!)} title="Up one folder" aria-label="Up one folder">↑</button>
    <code class="path" title={listing?.path}><span>{listing?.path || 'Folders bookman may use'}</span></code>
  </div>
  {#if error}<p class="error small">{error}</p>{/if}
  <ul class="dirs" aria-busy={loading}>
    {#each listing?.dirs ?? [] as d (d.path)}
      <li>
        <button type="button" class="ghost dir" onclick={() => open(d.path)}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
          <span class="name">{d.name}</span>
          {#if d.library}<span class="tag">library: {d.library}</span>{/if}
        </button>
      </li>
    {:else}
      {#if listing && !loading}<li class="faint small none">No sub-folders.</li>{/if}
    {/each}
  </ul>
  <div class="foot">
    <span class="faint small">
      {#if listing?.path}{listing.books_here ?? 0} {listing.books_here === 1 ? 'book' : 'books'} directly in this folder{/if}
    </span>
    <span class="spacer"></span>
    <button type="button" onclick={oncancel}>Cancel</button>
    <button type="button" class="primary" disabled={!listing?.path} onclick={() => onpick(listing!.path)}>Use this folder</button>
  </div>
</div>

<style>
  .picker { border: 1px solid var(--border); border-radius: 8px; background: var(--bg); display: flex; flex-direction: column; }
  .bar { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-bottom: 1px solid var(--border); }
  /* Long paths keep their end visible (ellipsis on the left); the inner span keeps the text itself left-to-right. */
  .path { font-size: 0.85rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; direction: rtl; text-align: left; }
  .path span { direction: ltr; unicode-bidi: isolate; }
  .dirs { list-style: none; margin: 0; padding: 4px; max-height: 300px; overflow: auto; min-height: 80px; }
  .dir { width: 100%; justify-content: flex-start; gap: 8px; border-radius: 6px; padding: 5px 8px; }
  .dir svg { color: var(--accent); flex: none; }
  .name { overflow: hidden; text-overflow: ellipsis; }
  .tag { margin-left: auto; font-size: 0.75rem; color: var(--fg-muted); background: var(--bg-sunken); border-radius: 999px; padding: 1px 8px; }
  .none { padding: 8px; }
  .foot { display: flex; align-items: center; gap: 8px; padding: 8px; border-top: 1px solid var(--border); flex-wrap: wrap; }
  .spacer { flex: 1; }
  .small { font-size: 0.82rem; }
</style>
