<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import type { Book, Library } from '../lib/types';
  import BookCard from '../components/BookCard.svelte';

  let continueReading = $state<Book[]>([]);
  let recent = $state<Book[]>([]);
  let libraries = $state<Library[]>([]);
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      [continueReading, recent, libraries] = await Promise.all([api.continueReading(), api.recentlyAdded(), api.libraries()]);
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
      <h2>No libraries configured</h2>
      <p>Add library folders to <code>bookman.toml</code> (the <code>libraries</code> list) and restart the server.</p>
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
          <a class="card lib" href="/library/{l.id}">
            <strong>{l.name}</strong>
            <span class="muted">{l.book_count} {l.book_count === 1 ? 'book' : 'books'}</span>
          </a>
        {/each}
      </div>
    </section>
  {/if}
</div>

<style>
  .libs { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
  .lib { display: flex; flex-direction: column; gap: 2px; color: var(--fg); }
  .lib:hover { border-color: var(--accent); text-decoration: none; }
</style>
