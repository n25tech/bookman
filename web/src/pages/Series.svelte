<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { humanize } from '../lib/format';
  import type { Book, Library, Series } from '../lib/types';
  import BookCard from '../components/BookCard.svelte';

  let { id }: { id: number } = $props();
  let data = $state<{ series: Series; library: Library; books: Book[] } | null>(null);
  let error = $state('');

  onMount(async () => {
    try { data = await api.series(id); } catch (e: any) { error = e.message; }
  });
  const readCount = $derived(data ? data.books.filter((b) => b.progress?.finished).length : 0);
</script>

<div class="page">
  {#if error}
    <div class="empty"><h2>Couldn't load the series</h2><p>{error}</p></div>
  {:else if !data}
    <div class="empty">Loading…</div>
  {:else}
    <p class="crumbs"><a href="/library/{data.library.id}">{data.library.name}</a> /</p>
    <h1>{humanize(data.series.name)}</h1>
    <p class="muted">{data.books.length} {data.books.length === 1 ? 'book' : 'books'} · {readCount} finished</p>
    {#if data.books.length === 0}
      <div class="empty"><h2>This series is empty</h2></div>
    {:else}
      <div class="grid">{#each data.books as b (b.id)}<BookCard book={b} />{/each}</div>
    {/if}
  {/if}
</div>

<style>
  .crumbs { margin: 0 0 4px; font-size: 0.9rem; }
</style>
