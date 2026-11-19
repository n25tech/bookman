<script lang="ts">
  import { onMount, tick } from 'svelte';
  import type { Book, Note } from '../lib/types';
  import type { Position } from './types';

  let { book, start, onposition, onready }: {
    book: Book; start: Position; onposition: (p: Position) => void; onready?: () => void;
  } = $props();

  const MODE_KEY = 'bookman:comic-mode';
  const FIT_KEY = 'bookman:comic-fit';
  function load<T extends string>(k: string, def: T): T { try { return (localStorage.getItem(k) as T) || def; } catch { return def; } }

  let mode = $state<'single' | 'scroll'>(load(MODE_KEY, 'single'));
  let fit = $state<'screen' | 'width'>(load(FIT_KEY, 'screen'));
  let page = $state(Math.min(Math.max(1, start.page), book.page_count));
  let pageInput = $state(String(page));
  let scroller: HTMLDivElement;
  let restoring = true;
  let loadedSingle = $state(false);

  const url = (n: number) => `/api/books/${book.id}/pages/${n}`;

  // Comics have no position within a page, except that the last page counts as
  // fully read (offset 1), so it can be auto-finished.
  function report() {
    pageInput = String(page);
    onposition({ page, offset: page === book.page_count ? 1 : 0 });
  }

  function preload(n: number) {
    for (const k of [n + 1, n + 2]) if (k <= book.page_count) { const i = new Image(); i.src = url(k); }
  }

  export async function goTo(n: number) {
    n = Math.min(Math.max(1, n), book.page_count);
    if (mode === 'single') {
      if (n !== page) loadedSingle = false;
      page = n;
      preload(n);
      scroller?.scrollTo(0, 0);
      report();
      return;
    }
    restoring = true;
    page = n;
    await tick();
    const img = scroller.querySelector<HTMLImageElement>(`img[data-page="${n}"]`);
    if (img) {
      img.loading = 'eager';
      scroller.scrollTop = img.offsetTop;
      if (!img.complete) await new Promise((r) => { img.onload = img.onerror = r; setTimeout(r, 8000); });
      scroller.scrollTop = img.offsetTop;
    }
    requestAnimationFrame(() => { restoring = false; report(); });
  }

  export function next() { if (page < book.page_count) goTo(page + 1); }
  export function prev() { if (page > 1) goTo(page - 1); }
  export function pageDown() {
    if (mode === 'single') {
      // Scroll a tall page first, then turn.
      if (scroller.scrollTop + scroller.clientHeight < scroller.scrollHeight - 4) scroller.scrollBy({ top: scroller.clientHeight * 0.9 });
      else next();
    } else scroller.scrollBy({ top: scroller.clientHeight * 0.9 });
  }
  export function pageUp() {
    if (mode === 'single') {
      if (scroller.scrollTop > 4) scroller.scrollBy({ top: -scroller.clientHeight * 0.9 });
      else prev();
    } else scroller.scrollBy({ top: -scroller.clientHeight * 0.9 });
  }
  export function flashNote(note: Note) { goTo(note.page); }

  function setMode(m: 'single' | 'scroll') {
    mode = m;
    try { localStorage.setItem(MODE_KEY, m); } catch {}
    tick().then(() => goTo(page));
  }
  function setFit(f: 'screen' | 'width') {
    fit = f;
    try { localStorage.setItem(FIT_KEY, f); } catch {}
  }

  function onScroll() {
    if (mode !== 'scroll' || restoring) return;
    const top = scroller.scrollTop + 1;
    const imgs = scroller.querySelectorAll<HTMLImageElement>('img[data-page]');
    let lo = 0, hi = imgs.length - 1;
    while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (imgs[mid].offsetTop <= top) lo = mid; else hi = mid - 1; }
    const n = lo + 1;
    if (n !== page) { page = n; report(); }
  }

  function zoneClick(e: MouseEvent) {
    if (mode !== 'single') return;
    const r = scroller.getBoundingClientRect();
    const x = (e.clientX - r.left) / r.width;
    if (x < 0.33) prev(); else if (x > 0.67) next();
  }

  function jump(e: SubmitEvent) { e.preventDefault(); const n = parseInt(pageInput, 10); if (n) goTo(n); }

  onMount(() => {
    goTo(page).then(() => onready?.());
    // Keep the current page in place when the viewer is resized (scroll mode).
    const ro = new ResizeObserver(() => {
      if (mode !== 'scroll') return;
      const img = scroller.querySelector<HTMLImageElement>(`img[data-page="${page}"]`);
      if (img) { restoring = true; scroller.scrollTop = img.offsetTop; requestAnimationFrame(() => (restoring = false)); }
    });
    ro.observe(scroller);
    return () => ro.disconnect();
  });
</script>

<div class="comic">
  <div class="tools">
    <form class="pageform" onsubmit={jump}>
      <input aria-label="Page" inputmode="numeric" bind:value={pageInput} size="4" />
      <span class="faint">/ {book.page_count}</span>
    </form>
    <span class="sep"></span>
    <div class="seg">
      <button class:on={mode === 'single'} onclick={() => setMode('single')}>Single</button>
      <button class:on={mode === 'scroll'} onclick={() => setMode('scroll')}>Scroll</button>
    </div>
    {#if mode === 'single'}
      <div class="seg">
        <button class:on={fit === 'screen'} onclick={() => setFit('screen')} title="Fit the page on screen">Fit page</button>
        <button class:on={fit === 'width'} onclick={() => setFit('width')} title="Fit the page width">Fit width</button>
      </div>
    {/if}
  </div>
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="scroller {mode} fit-{fit}" bind:this={scroller} onscroll={onScroll} onclick={zoneClick} tabindex="-1">
    {#if mode === 'single'}
      {#key page}
        <img src={url(page)} alt="Page {page}" class:loaded={loadedSingle} onload={() => (loadedSingle = true)} />
      {/key}
      {#if !loadedSingle}<div class="spinner faint">Loading page {page}…</div>{/if}
    {:else}
      {#each Array.from({ length: book.page_count }, (_, i) => i + 1) as n (n)}
        <img src={url(n)} alt="Page {n}" data-page={n} loading={Math.abs(n - page) <= 2 ? 'eager' : 'lazy'} />
      {/each}
    {/if}
  </div>
</div>

<style>
  .comic { height: 100%; display: flex; flex-direction: column; background: var(--viewer-bg); }
  .tools { display: flex; align-items: center; gap: 6px; padding: 4px 8px; border-bottom: 1px solid var(--border); background: var(--bg-elev); flex-wrap: wrap; }
  .pageform { display: flex; align-items: center; gap: 6px; }
  .pageform input { width: 4.5em; text-align: center; padding: 3px 6px; }
  .sep { width: 1px; height: 20px; background: var(--border); margin: 0 4px; }
  .seg { display: inline-flex; border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
  .seg button { border: 0; border-radius: 0; padding: 3px 10px; background: transparent; color: var(--fg-muted); font-size: 0.85rem; }
  .seg button.on { background: var(--accent-soft); color: var(--fg); }
  .scroller { flex: 1; min-height: 0; overflow: auto; outline: none; position: relative; }
  .single { display: flex; justify-content: center; align-items: flex-start; cursor: pointer; user-select: none; }
  .single img { display: block; margin: 0 auto; opacity: 0; transition: opacity 0.1s; }
  .single img.loaded { opacity: 1; }
  .single.fit-screen img { max-width: 100%; max-height: 100%; object-fit: contain; align-self: center; }
  .single.fit-screen { align-items: center; height: 100%; }
  .single.fit-width img { width: 100%; max-width: 1400px; height: auto; }
  .spinner { position: absolute; inset: 0; display: grid; place-items: center; pointer-events: none; }
  .scroll img { display: block; width: 100%; max-width: 1100px; margin: 0 auto 4px; min-height: 200px; aspect-ratio: auto 2 / 3; background: var(--bg-sunken); }
</style>
