<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { humanize } from '../lib/format';
  import { refreshLibraries } from '../lib/libraries.svelte';
  import type { Book, Folder, Library } from '../lib/types';
  import BookCard from '../components/BookCard.svelte';
  import { session } from '../lib/session.svelte';

  let continueReading = $state<Book[]>([]);
  let recent = $state<Book[]>([]);
  let libraries = $state<Library[]>([]);
  let folders = $state<Record<number, Folder[]>>({});
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      [continueReading, recent, libraries] = await Promise.all([api.continueReading(), api.recentlyAdded(), refreshLibraries()]);
      // Top-level folders of each library, as shortcuts.
      for (const l of libraries) {
        api.browse(l.id, '').then((r) => { folders[l.id] = r.folders; }).catch(() => {});
      }
    } catch (e: any) {
      error = e.message;
    }
    loading = false;
  });
</script>

<div class="page">
  {#if loading}
    <div class="empty">Loading…</div>
  {:else if error}
    <div class="empty"><h2>Something went wrong</h2><p>{error}</p></div>
  {:else if libraries.length === 0}
    <div class="empty">
      <h2>No libraries yet</h2>
      {#if session.user?.is_admin}
        <p>Add a folder of books to get started.</p>
        <a class="btn primary" href="/admin">Add a library</a>
      {:else}
        <p>Ask an admin to add a library.</p>
      {/if}
    </div>
  {:else}
    {#if continueReading.length}
      <section>
        <h2>Continue reading</h2>
        <div class="shelf">
          {#each continueReading as b (b.id)}<BookCard book={b} href="/read/{b.id}" />{/each}
        </div>
      </section>
    {/if}
    {#if recent.length}
      <section>
        <h2>Recently added</h2>
        <div class="shelf">
          {#each recent as b (b.id)}<BookCard book={b} />{/each}
        </div>
      </section>
    {:else}
      <section class="empty">
        <h2>Nothing here yet</h2>
        <p>The library is being scanned. Books appear as they are found.</p>
      </section>
    {/if}
    <section>
      <h2>Libraries</h2>
      <div class="libs">
        {#each libraries as l (l.id)}
          <div class="card lib">
            <a class="libname" href="/library/{l.id}">
              <strong>{l.name}</strong>
              <span class="muted">{l.book_count} {l.book_count === 1 ? 'book' : 'books'}</span>
            </a>
            {#if folders[l.id]?.length}
              <div class="chips">
                {#each folders[l.id] as f (f.path)}
                  <a class="chip" href="/library/{l.id}?path={encodeURIComponent(f.path)}">{humanize(f.name)} <span class="faint">{f.book_count}</span></a>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    </section>
  {/if}
</div>

<style>
  .libs { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; align-items: start; }
  .lib { display: flex; flex-direction: column; gap: 10px; }
  .libname { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; color: var(--fg); }
  .libname:hover { text-decoration: none; color: var(--accent); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { font-size: 0.82rem; padding: 3px 10px; border-radius: 999px; background: var(--bg-sunken); color: var(--fg); border: 1px solid var(--border); max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .chip:hover { border-color: var(--accent); text-decoration: none; }
</style>
