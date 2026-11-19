<script lang="ts">
  import { coverUrl } from '../lib/api';
  let { bookId, etag, title, format }: { bookId: number; etag: string | null; title: string; format: string } = $props();
  let failed = $state(false);
  const url = $derived(coverUrl(bookId, etag));
</script>

<div class="cover">
  {#if url && !failed}
    <img src={url} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
  {:else}
    <div class="placeholder">
      <span class="t">{title}</span>
      <span class="f">{format.toUpperCase()}</span>
    </div>
  {/if}
</div>

<style>
  .cover { aspect-ratio: 2 / 3; background: var(--cover-bg); border-radius: 6px; overflow: hidden; box-shadow: var(--shadow); }
  img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .placeholder { height: 100%; display: flex; flex-direction: column; justify-content: space-between; padding: 12px; color: var(--fg-muted); }
  .t { font-weight: 600; font-size: 0.9rem; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 6; line-clamp: 6; -webkit-box-orient: vertical; }
  .f { font-size: 0.7rem; letter-spacing: 0.08em; color: var(--fg-faint); }
</style>
