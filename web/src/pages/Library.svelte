<script lang="ts">
  import { api } from '../lib/api';
  import { router } from '../lib/router.svelte';
  import { humanize } from '../lib/format';
  import type { Book, BrowseResult, Library } from '../lib/types';
  import BookCard from '../components/BookCard.svelte';
  import FolderCard from '../components/FolderCard.svelte';

  let { id }: { id: number } = $props();

  // URL state: ?path=a/b for the folder, ?view=all for the flat list, plus filters.
  const query = $derived(router.query());
  const view = $derived<'folders' | 'all'>(query.get('view') === 'all' ? 'all' : 'folders');
  const path = $derived(query.get('path') ?? '');

  let q = $state(router.query().get('q') ?? '');
  let format = $state(router.query().get('format') ?? '');
  let status = $state(router.query().get('status') ?? '');
  let sort = $state(router.query().get('sort') ?? 'title');

  let library = $state<Library | null>(null);
  let browse = $state<BrowseResult | null>(null);
  let books = $state<Book[]>([]); // flat list: "All books", or search results
  let total = $state(0);
  let loading = $state(true);
  let error = $state('');
  let shown = $state(120);
  let sentinel = $state<HTMLElement>();

  const searching = $derived(view === 'all' || q.trim() !== '' || format !== '' || status !== '');

  function href(next: { path?: string; view?: string }): string {
    const p = new URLSearchParams();
    const v = next.view ?? view;
    if (v === 'all') p.set('view', 'all');
    const np = next.path ?? path;
    if (v !== 'all' && np) p.set('path', np);
    const s = p.toString();
    return `/library/${id}${s ? '?' + s : ''}`;
  }

  let seq = 0;
  async function load() {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      if (searching) {
        const params: Record<string, string> = { sort };
        if (q.trim()) params.q = q.trim();
        if (format) params.format = format;
        if (status) params.status = status;
        const r = await api.libraryBooks(id, params);
        if (mine !== seq) return;
        library = r.library;
        // In the folder view, a search covers the current folder and everything below it.
        const prefix = view === 'folders' && path ? path + '/' : '';
        books = prefix ? r.books.filter((b) => b.rel_path.startsWith(prefix)) : r.books;
        total = books.length;
        shown = 120;
      }
      if (view === 'folders') {
        const r = await api.browse(id, path);
        if (mine !== seq) return;
        browse = r;
        library = r.library;
      }
    } catch (e: any) {
      if (mine === seq) error = e.message;
    }
    if (mine === seq) loading = false;
  }

  // Reload when the folder or view in the URL changes (links, back/forward).
  $effect(() => { view; path; load(); });

  let debounce: number | undefined;
  function onFilter() {
    clearTimeout(debounce);
    debounce = window.setTimeout(load, 250);
  }
  function clearFilters() { q = ''; format = ''; status = ''; load(); }

  $effect(() => {
    if (!sentinel) return;
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) shown += 120;
    }, { rootMargin: '800px' });
    io.observe(sentinel);
    return () => io.disconnect();
  });

  const title = $derived(view === 'folders' && path ? humanize(path.split('/').pop()!) : library?.name ?? 'Library');
</script>

<div class="page">
  <div class="head">
    <div class="titles">
      {#if view === 'folders' && browse && browse.breadcrumbs.length}
        <nav class="crumbs" aria-label="Folder">
          <a href={href({ path: '' })}>{library?.name}</a>
          {#each browse.breadcrumbs.slice(0, -1) as c (c.path)}
            <span class="sep">/</span><a href={href({ path: c.path })}>{humanize(c.name)}</a>
          {/each}
        </nav>
      {/if}
      <h1>{title}</h1>
    </div>
    <div class="seg" role="tablist">
      <a role="tab" class:on={view === 'folders'} aria-selected={view === 'folders'} href={href({ view: 'folders', path: '' })}>Folders</a>
      <a role="tab" class:on={view === 'all'} aria-selected={view === 'all'} href={href({ view: 'all' })}>All books</a>
    </div>
  </div>

  <div class="filters row">
    <input type="search" placeholder={view === 'folders' && path ? `Search in ${title}` : 'Search title or author'} bind:value={q} oninput={onFilter} aria-label="Search" />
    <select bind:value={format} onchange={load} aria-label="Format">
      <option value="">All formats</option><option value="pdf">PDF</option><option value="cbz">CBZ</option><option value="cbr">CBR</option>
    </select>
    <select bind:value={status} onchange={load} aria-label="Status">
      <option value="">Any status</option><option value="unread">Unread</option><option value="reading">Reading</option><option value="finished">Finished</option>
    </select>
    {#if searching}
      <select bind:value={sort} onchange={load} aria-label="Sort">
        <option value="title">Title</option><option value="added">Recently added</option><option value="last_read">Last read</option>
      </select>
      <span class="faint">{total} {total === 1 ? 'book' : 'books'}</span>
      {#if view === 'folders'}<button class="ghost" onclick={clearFilters}>Clear</button>{/if}
    {/if}
  </div>

  {#if error}
    <div class="empty"><h2>Couldn't load the library</h2><p>{error}</p><button onclick={load}>Retry</button></div>
  {:else if loading && !browse && !books.length}
    <div class="empty">Loading…</div>
  {:else if searching}
    {#if books.length === 0}
      <div class="empty">
        {#if q || format || status}<h2>No matches</h2><p>Try a different search or filter.</p>
        {:else}<h2>No books yet</h2><p>Books show up here once the scan finds them.</p>{/if}
      </div>
    {:else}
      <div class="grid">{#each books.slice(0, shown) as b (b.id)}<BookCard book={b} />{/each}</div>
      {#if shown < books.length}<div bind:this={sentinel} class="more faint">Loading more…</div>{/if}
    {/if}
  {:else if browse}
    {#if browse.folders.length === 0 && browse.books.length === 0}
      <div class="empty"><h2>No books yet</h2><p>Books show up here once the scan finds them.</p></div>
    {:else}
      {#if browse.folders.length}
        <section>
          {#if browse.books.length}<h2>Folders</h2>{/if}
          <div class="grid">
            {#each browse.folders as f (f.path)}<FolderCard folder={f} href={href({ path: f.path })} />{/each}
          </div>
        </section>
      {/if}
      {#if browse.books.length}
        <section>
          {#if browse.folders.length}<h2>Books</h2>{/if}
          <div class="grid">{#each browse.books as b (b.id)}<BookCard book={b} />{/each}</div>
        </section>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .head { display: flex; align-items: flex-end; gap: 16px; flex-wrap: wrap; margin-bottom: 12px; }
  .titles { min-width: 0; flex: 1; }
  .head h1 { margin: 0; overflow-wrap: anywhere; }
  .crumbs { font-size: 0.9rem; color: var(--fg-muted); margin-bottom: 2px; display: flex; flex-wrap: wrap; gap: 4px; }
  .sep { color: var(--fg-faint); }
  .seg { display: inline-flex; border: 1px solid var(--border); border-radius: 7px; overflow: hidden; }
  .seg a { padding: 6px 12px; color: var(--fg-muted); }
  .seg a:hover { text-decoration: none; color: var(--fg); }
  .seg a.on { background: var(--accent-soft); color: var(--fg); }
  .filters { margin-bottom: 20px; }
  .filters input[type='search'] { flex: 1 1 220px; max-width: 360px; }
  .more { text-align: center; padding: 24px; }
</style>
