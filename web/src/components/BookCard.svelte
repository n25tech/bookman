<script lang="ts">
  import type { Book } from '../lib/types';
  import { percent } from '../lib/format';
  import Cover from './Cover.svelte';
  let { book, href }: { book: Book; href?: string } = $props();
  const p = $derived(book.progress);
</script>

<a class="card-link" href={href ?? `/book/${book.id}`} title={book.title}>
  <div class="wrap">
    <Cover bookId={book.id} etag={book.cover_etag} title={book.title} format={book.format} />
    {#if p?.finished}
      <span class="badge done" title="Finished">✓</span>
    {:else if p}
      <div class="bar" title="{percent(p.percent)} read"><div style="width: {percent(p.percent)}"></div></div>
    {/if}
    {#if book.status === 'error'}<span class="badge err" title={book.error_msg}>!</span>{/if}
  </div>
  <div class="title">{book.title}</div>
  <div class="meta">
    {#if p && !p.finished}{percent(p.percent)} · {/if}{book.author || book.format.toUpperCase()}
  </div>
</a>

<style>
  .card-link { display: block; color: inherit; min-width: 0; }
  .card-link:hover { text-decoration: none; }
  .card-link:hover .title { color: var(--accent); }
  .wrap { position: relative; transition: transform 0.12s; }
  .card-link:hover .wrap { transform: translateY(-2px); }
  .title { margin-top: 8px; font-size: 0.88rem; font-weight: 550; line-height: 1.3; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .meta { font-size: 0.78rem; color: var(--fg-faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .bar { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 0.35); border-radius: 0 0 6px 6px; overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); }
  .badge { position: absolute; top: 6px; right: 6px; width: 22px; height: 22px; border-radius: 50%; display: grid; place-items: center; font-size: 0.75rem; font-weight: 700; }
  .done { background: var(--ok); color: #fff; }
  .err { background: var(--danger); color: #fff; left: 6px; right: auto; }
</style>
