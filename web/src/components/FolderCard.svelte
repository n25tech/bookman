<script lang="ts">
  import { coverUrl } from '../lib/api';
  import { humanize } from '../lib/format';
  import type { Folder } from '../lib/types';
  let { folder, href }: { folder: Folder; href: string } = $props();
  const url = $derived(coverUrl(folder.cover_book_id, folder.cover_etag));
  const label = $derived(humanize(folder.name));
</script>

<a class="folder" {href} title={label}>
  <div class="stack">
    <div class="sheet s2"></div>
    <div class="sheet s1"></div>
    <div class="front">
      {#if url}<img src={url} alt="" loading="lazy" decoding="async" />{/if}
      <div class="tab">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
        {folder.book_count}
      </div>
    </div>
  </div>
  <div class="name">{label}</div>
  <div class="meta">
    {folder.book_count} {folder.book_count === 1 ? 'book' : 'books'}{#if folder.finished_count} · {folder.finished_count} read{/if}{#if folder.reading_count} · {folder.reading_count} in progress{/if}
  </div>
</a>

<style>
  .folder { display: block; color: inherit; min-width: 0; }
  .folder:hover { text-decoration: none; }
  .folder:hover .name { color: var(--accent); }
  .stack { position: relative; aspect-ratio: 2 / 3; margin: 6px 6px 0 0; transition: transform 0.12s; }
  .folder:hover .stack { transform: translateY(-2px); }
  .sheet, .front { position: absolute; inset: 0; border-radius: 6px; }
  .sheet { background: var(--cover-bg); border: 1px solid var(--border); }
  .s2 { transform: translate(6px, -6px); opacity: 0.55; }
  .s1 { transform: translate(3px, -3px); opacity: 0.8; }
  .front { overflow: hidden; background: var(--accent-soft); box-shadow: var(--shadow); display: grid; place-items: center; }
  .front img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .tab {
    position: absolute; left: 6px; bottom: 6px; display: flex; align-items: center; gap: 4px;
    padding: 2px 8px; border-radius: 999px; font-size: 0.75rem; font-weight: 600;
    background: rgb(0 0 0 / 0.62); color: #fff;
  }
  .name { margin-top: 8px; font-size: 0.9rem; font-weight: 600; line-height: 1.3; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .meta { font-size: 0.78rem; color: var(--fg-faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
