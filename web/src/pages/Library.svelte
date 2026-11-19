<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { router } from '../lib/router.svelte';
  import type { Book, Library, Series } from '../lib/types';
  import BookCard from '../components/BookCard.svelte';
  import Cover from '../components/Cover.svelte';

  let { id }: { id: number } = $props();

  const q0 = router.query();
  let view = $state<'books' | 'series'>((q0.get('view') as any) || 'books');
  let q = $state(q0.get('q') ?? '');
  let format = $state(q0.get('format') ?? '');
  let status = $state(q0.get('status') ?? '');
  let sort = $state(q0.get('sort') ?? 'title');

  let library = $state<Library | null>(null);
  let books = $state<Book[]>([]);
  let series = $state<Series[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state('');
  let shown = $state(120); // render in chunks: big comic libraries have thousands of books
  let sentinel = $state<HTMLElement>();

  let seq = 0;
  async function load() {
    const mine = ++seq;
    loading = true;
    error = '';
    const params: Record<string, string> = { sort };
    if (q) params.q = q;
    if (format) params.format = format;
    if (status) params.status = status;
    const url = new URLSearchParams({ ...params, ...(view === 'series' ? { view } : {}) });
    history.replaceState(null, '', `/library/${id}?${url}`);
    try {
      if (view === 'series') {
        const s = await api.librarySeries(id);
        if (mine !== seq) return;
        series = s;
        if (!library) library = (await api.libraryBooks(id, { limit: '1' })).library;
      } else {
        const r = await api.libraryBooks(id, params);
        if (mine !== seq) return;
        library = r.library;
        books = r.books;
        total = r.total;
        shown = 120;
      }
    } catch (e: any) {
      error = e.message;
    }
    loading = false;
  }

  let debounce: number | undefined;
  function onSearch() {
    clearTimeout(debounce);
    debounce = window.setTimeout(load, 250);
  }

  onMount(load);

  // Render more covers as the end of the grid comes into view.
  $effect(() => {
    if (!sentinel) return;
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) shown += 120;
    }, { rootMargin: '800px' });
    io.observe(sentinel);
    return () => io.disconnect();
  });

  const filtered = $derived(view === 'series' && q ? series.filter((s) => s.name.toLowerCase().includes(q.toLowerCase())) : series);
</script>

<div class="page">
  <div class="head">
    <h1>{library?.name ?? 'Library'}</h1>
    <div class="seg" role="tablist">
      <button role="tab" class:on={view === 'books'} aria-selected={view === 'books'} onclick={() => { view = 'books'; load(); }}>Books</button>
      <button role="tab" class:on={view === 'series'} aria-selected={view === 'series'} onclick={() => { view = 'series'; load(); }}>Series</button>
    </div>
  </div>

  <div class="filters row">
    <input type="search" placeholder="Search title or author" bind:value={q} oninput={onSearch} aria-label="Search" />
    {#if view === 'books'}
      <select bind:value={format} onchange={load} aria-label="Format">
        <option value="">All formats</option><option value="pdf">PDF</option><option value="cbz">CBZ</option><option value="cbr">CBR</option>
      </select>
      <select bind:value={status} onchange={load} aria-label="Status">
        <option value="">Any status</option><option value="unread">Unread</option><option value="reading">Reading</option><option value="finished">Finished</option>
      </select>
      <select bind:value={sort} onchange={load} aria-label="Sort">
        <option value="title">Title</option><option value="added">Recently added</option><option value="last_read">Last read</option>
      </select>
      <span class="faint">{total} {total === 1 ? 'book' : 'books'}</span>
    {/if}
  </div>

  {#if error}
    <div class="empty"><h2>Couldn't load the library</h2><p>{error}</p><button onclick={load}>Retry</button></div>
  {:else if loading && !books.length && !series.length}
    <div class="empty">Loading…</div>
  {:else if view === 'books'}
    {#if books.length === 0}
      <div class="empty">
        {#if q || format || status}<h2>No matches</h2><p>Try a different search or filter.</p>
        {:else}<h2>No books yet</h2><p>Books show up here once the scan finds them.</p>{/if}
      </div>
    {:else}
      <div class="grid">
        {#each books.slice(0, shown) as b (b.id)}<BookCard book={b} />{/each}
      </div>
      {#if shown < books.length}<div bind:this={sentinel} class="more faint">Loading more…</div>{/if}
    {/if}
  {:else}
    {#if filtered.length === 0}
      <div class="empty"><h2>No series</h2></div>
    {:else}
      <div class="grid">
        {#each filtered as s (s.id)}
          <a class="series" href="/series/{s.id}">
            <Cover bookId={s.cover_book_id ?? 0} etag={s.cover_etag} title={s.name} format="series" />
            <div class="title">{s.name}</div>
            <div class="faint small">{s.book_count} {s.book_count === 1 ? 'book' : 'books'}</div>
          </a>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .head { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; margin-bottom: 12px; }
  .head h1 { margin: 0; }
  .seg { display: inline-flex; border: 1px solid var(--border); border-radius: 7px; overflow: hidden; }
  .seg button { border: 0; border-radius: 0; background: transparent; color: var(--fg-muted); }
  .seg button.on { background: var(--accent-soft); color: var(--fg); }
  .filters { margin-bottom: 20px; }
  .filters input[type='search'] { flex: 1 1 220px; max-width: 360px; }
  .more { text-align: center; padding: 24px; }
  .series { color: inherit; display: block; min-width: 0; }
  .series:hover { text-decoration: none; }
  .series:hover .title { color: var(--accent); }
  .title { margin-top: 8px; font-size: 0.88rem; font-weight: 550; line-height: 1.3; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .small { font-size: 0.78rem; }
</style>
