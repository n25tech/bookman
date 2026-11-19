<script lang="ts">
  import { onMount } from 'svelte';
  import 'pdfjs-dist/web/pdf_viewer.css';
  import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';
  import type { Book, Note } from '../lib/types';
  import type { NewAnchor, Position } from './types';
  import { offsetsToRange, rangeRects, rangeToOffsets } from './anchors';

  let {
    book, start, notes, onposition, onaddnote, onready, onerror,
  }: {
    book: Book;
    start: Position;
    notes: Note[];
    onposition: (p: Position) => void;
    onaddnote: (a: NewAnchor) => void;
    onready?: () => void;
    onerror?: (msg: string) => void;
  } = $props();

  let container: HTMLDivElement;
  let viewerEl: HTMLDivElement;
  let viewer: any = null;
  let eventBus: any = null;
  let pdfDoc: any = null;

  let fitWidth = $state(true);
  let scalePct = $state(100);
  let pageInput = $state(String(start.page));
  let searchOpen = $state(false);
  let query = $state('');
  let matches = $state<{ current: number; total: number } | null>(null);
  let selBtn = $state<{ x: number; y: number; anchor: NewAnchor } | null>(null);

  // The logical reading position. Layout changes (resize, zoom, pages loading)
  // re-apply it, so the reader stays on the same spot.
  let anchor: Position = { ...start };
  let restoring = true;
  let userScrolled = false;

  function pageDiv(i: number): HTMLElement | null {
    return viewer?.getPageView(i)?.div ?? null;
  }

  function computePosition(): Position {
    const n = viewer.pagesCount;
    const top = container.scrollTop;
    if (top + container.clientHeight >= container.scrollHeight - 2 && top > 0) return { page: n, offset: 1 };
    let lo = 0, hi = n - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (pageDiv(mid)!.offsetTop <= top + 1) lo = mid; else hi = mid - 1;
    }
    const div = pageDiv(lo)!;
    const offset = (top - div.offsetTop) / Math.max(1, div.offsetHeight);
    return { page: lo + 1, offset: Math.min(1, Math.max(0, offset)) };
  }

  function applyAnchor() {
    const div = pageDiv(anchor.page - 1);
    if (!div) return;
    container.scrollTop = div.offsetTop + anchor.offset * div.offsetHeight;
  }

  function waitRendered(page: number, timeout = 8000): Promise<void> {
    return new Promise((resolve) => {
      const pv = viewer.getPageView(page - 1);
      if (pv?.renderingState === 3 /* FINISHED */) return resolve();
      const done = () => { eventBus.off('pagerendered', on); clearTimeout(t); resolve(); };
      const on = (e: any) => { if (e.pageNumber === page) done(); };
      const t = setTimeout(done, timeout);
      eventBus.on('pagerendered', on);
    });
  }

  export async function goTo(page: number, offset = 0) {
    if (!viewer) return;
    page = Math.min(Math.max(1, page), viewer.pagesCount);
    restoring = true;
    userScrolled = false;
    anchor = { page, offset };
    applyAnchor();
    await waitRendered(page);
    applyAnchor();
    requestAnimationFrame(() => {
      restoring = false;
      pageInput = String(page);
      onposition(anchor);
    });
  }

  export function next() { goTo(Math.min(viewer.pagesCount, computePosition().page + 1)); }
  export function prev() {
    const p = computePosition();
    goTo(p.offset > 0.05 ? p.page : Math.max(1, p.page - 1));
  }
  export function pageDown() { container.scrollBy({ top: container.clientHeight * 0.9 }); }
  export function pageUp() { container.scrollBy({ top: -container.clientHeight * 0.9 }); }

  export function flashNote(note: Note) {
    goTo(note.page, 0).then(() => {
      setTimeout(() => {
        const els = container.querySelectorAll<HTMLElement>(`[data-note="${note.id}"]`);
        if (els.length) {
          els[0].scrollIntoView({ block: 'center' });
          els.forEach((el) => { el.classList.remove('flash'); void el.offsetWidth; el.classList.add('flash'); });
        }
      }, 150);
    });
  }

  // Highlights for saved anchors, drawn over the text layer as page-relative boxes.
  function drawHighlights(pageNumber: number) {
    const pv = viewer?.getPageView(pageNumber - 1);
    if (!pv?.div) return;
    pv.div.querySelector('.bm-highlights')?.remove();
    const layer: HTMLElement | null = pv.div.querySelector('.textLayer');
    const pageNotes = notes.filter((n) => n.page === pageNumber && n.anchor_start != null && n.anchor_end != null);
    if (!layer || !pageNotes.length) return;
    const box = document.createElement('div');
    box.className = 'bm-highlights';
    for (const n of pageNotes) {
      const r = offsetsToRange(layer, n.anchor_start!, n.anchor_end!);
      if (!r) continue;
      for (const rect of rangeRects(r, pv.div)) {
        const d = document.createElement('div');
        d.className = `bm-hl bm-hl-${n.color || 'yellow'}`;
        d.dataset.note = String(n.id);
        Object.assign(d.style, { left: `${rect.x * 100}%`, top: `${rect.y * 100}%`, width: `${rect.w * 100}%`, height: `${rect.h * 100}%` });
        box.appendChild(d);
      }
    }
    pv.div.appendChild(box);
  }

  function redrawAll() {
    if (!viewer) return;
    for (let i = 1; i <= viewer.pagesCount; i++) {
      if (viewer.getPageView(i - 1)?.div?.querySelector('.textLayer')) drawHighlights(i);
    }
  }

  $effect(() => { notes; redrawAll(); });

  function onSelection() {
    const sel = document.getSelection();
    if (!sel || sel.isCollapsed || !sel.rangeCount) { selBtn = null; return; }
    const range = sel.getRangeAt(0);
    const startEl = range.startContainer.parentElement;
    const layer = startEl?.closest<HTMLElement>('.textLayer');
    const page = layer?.closest<HTMLElement>('.page');
    if (!layer || !page || !container.contains(layer)) { selBtn = null; return; }
    const offs = rangeToOffsets(layer, range);
    const quote = sel.toString().trim();
    if (!offs || !quote) { selBtn = null; return; }
    const r = range.getBoundingClientRect();
    const host = container.getBoundingClientRect();
    selBtn = {
      x: Math.min(host.width - 110, Math.max(4, r.left - host.left + r.width / 2 - 50)),
      y: Math.max(4, r.top - host.top - 40),
      anchor: { page: Number(page.dataset.pageNumber), quote, anchor_start: offs.start, anchor_end: offs.end },
    };
  }

  function addNoteFromSelection() {
    if (!selBtn) return;
    onaddnote(selBtn.anchor);
    document.getSelection()?.removeAllRanges();
    selBtn = null;
  }

  function setZoom(mult: number | 'fit') {
    if (!viewer) return;
    anchor = viewer.pagesCount ? computePosition() : anchor;
    restoring = true;
    if (mult === 'fit') { fitWidth = true; viewer.currentScaleValue = 'page-width'; }
    else { fitWidth = false; viewer.currentScale = Math.min(5, Math.max(0.25, viewer.currentScale * mult)); }
  }

  function find(again = false, previous = false) {
    eventBus?.dispatch('find', {
      source: null, type: again ? 'again' : '', query, caseSensitive: false, entireWord: false,
      highlightAll: true, findPrevious: previous, matchDiacritics: false,
    });
  }
  export function openSearch() { searchOpen = true; setTimeout(() => document.getElementById('pdf-search')?.focus(), 0); }

  function jumpToInput(e: SubmitEvent) {
    e.preventDefault();
    const n = parseInt(pageInput, 10);
    if (n) goTo(n);
  }

  onMount(() => {
    let destroyed = false;
    let ro: ResizeObserver | null = null;
    let raf = 0;
    let lastW = 0, lastH = 0;

    const onScroll = () => {
      if (restoring || raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        if (!viewer?.pagesCount || restoring) return;
        anchor = computePosition();
        pageInput = String(anchor.page);
        onposition(anchor);
      });
    };
    const markUser = () => { userScrolled = true; };

    (async () => {
      try {
        const pdfjsLib: any = await import('pdfjs-dist');
        (globalThis as any).pdfjsLib = pdfjsLib;
        pdfjsLib.GlobalWorkerOptions.workerSrc = workerUrl;
        const { EventBus, PDFLinkService, PDFFindController, PDFViewer } = await import('pdfjs-dist/web/pdf_viewer.mjs' as any);
        if (destroyed) return;

        eventBus = new EventBus();
        const linkService = new PDFLinkService({ eventBus });
        const findController = new PDFFindController({ eventBus, linkService });
        viewer = new PDFViewer({ container, viewer: viewerEl, eventBus, linkService, findController, textLayerMode: 1, removePageBorders: false });
        linkService.setViewer(viewer);

        eventBus.on('pagesinit', () => {
          viewer.currentScaleValue = 'page-width';
          goTo(start.page, start.offset).then(() => onready?.());
        });
        eventBus.on('scalechanging', (e: any) => {
          scalePct = Math.round(e.scale * 100);
          requestAnimationFrame(() => { applyAnchor(); requestAnimationFrame(() => { restoring = false; }); });
        });
        // Pages above the anchor get their real size as they load; keep the
        // anchor in place until the reader scrolls by themselves.
        eventBus.on('pagerendered', (e: any) => {
          if (!userScrolled && !restoring && e.pageNumber <= anchor.page + 1) applyAnchor();
          // A re-render (zoom, resize) resets the page box and drops our overlay,
          // while the text layer is kept; redraw from it.
          drawHighlights(e.pageNumber);
        });
        eventBus.on('textlayerrendered', (e: any) => drawHighlights(e.pageNumber));
        eventBus.on('updatefindmatchescount', (e: any) => { matches = e.matchesCount; });
        eventBus.on('updatefindcontrolstate', (e: any) => { if (e.matchesCount) matches = e.matchesCount; });

        const task = pdfjsLib.getDocument({
          url: `/api/books/${book.id}/file`,
          withCredentials: true,
          rangeChunkSize: 256 * 1024,
          disableAutoFetch: true,
          disableStream: true,
        });
        pdfDoc = await task.promise;
        if (destroyed) { pdfDoc.destroy(); return; }
        viewer.setDocument(pdfDoc);
        linkService.setDocument(pdfDoc, null);
      } catch (e: any) {
        console.error(e);
        onerror?.(e?.message ?? String(e));
      }
    })();

    container.addEventListener('scroll', onScroll, { passive: true });
    for (const ev of ['wheel', 'touchstart', 'keydown', 'mousedown']) container.addEventListener(ev, markUser, { passive: true });
    document.addEventListener('selectionchange', onSelection);

    // Re-fit on any size change of the viewer (notes panel drag, window resize,
    // phone rotation) and put the reader back on the same spot.
    ro = new ResizeObserver((entries) => {
      const r = entries[0].contentRect;
      if (!viewer?.pagesCount || (Math.abs(r.width - lastW) < 1 && Math.abs(r.height - lastH) < 1)) return;
      lastW = r.width; lastH = r.height;
      restoring = true;
      if (fitWidth) viewer.currentScaleValue = 'page-width';
      requestAnimationFrame(() => { applyAnchor(); requestAnimationFrame(() => { restoring = false; }); });
    });
    ro.observe(container);

    return () => {
      destroyed = true;
      ro?.disconnect();
      document.removeEventListener('selectionchange', onSelection);
      viewer?.setDocument(null);
      pdfDoc?.destroy();
    };
  });
</script>

<div class="pdf">
  <div class="tools">
    <form class="pageform" onsubmit={jumpToInput}>
      <input aria-label="Page" inputmode="numeric" bind:value={pageInput} size="4" />
      <span class="faint">/ {book.page_count}</span>
    </form>
    <span class="sep"></span>
    <button class="ghost icon" onclick={() => setZoom(1 / 1.15)} title="Zoom out" aria-label="Zoom out">−</button>
    <button class="ghost zoom" onclick={() => setZoom('fit')} title="Fit width">{fitWidth ? 'Fit' : `${scalePct}%`}</button>
    <button class="ghost icon" onclick={() => setZoom(1.15)} title="Zoom in" aria-label="Zoom in">+</button>
    <span class="sep"></span>
    {#if searchOpen}
      <form class="search" onsubmit={(e) => { e.preventDefault(); find(true); }}>
        <input id="pdf-search" type="search" placeholder="Find in book" bind:value={query} oninput={() => find(false)}
          onkeydown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); searchOpen = false; query = ''; find(false); } else if (e.key === 'Enter' && e.shiftKey) { e.preventDefault(); find(true, true); } }} />
        <button type="button" class="ghost icon" onclick={() => find(true, true)} aria-label="Previous match">↑</button>
        <button type="submit" class="ghost icon" aria-label="Next match">↓</button>
        {#if query && matches}<span class="faint small">{matches.total ? `${matches.current}/${matches.total}` : 'no matches'}</span>{/if}
      </form>
    {:else}
      <button class="ghost icon" onclick={openSearch} title="Find (Ctrl+F)" aria-label="Find">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>
      </button>
    {/if}
  </div>
  <div class="stage">
    <div class="container" bind:this={container} tabindex="-1">
      <div class="pdfViewer" bind:this={viewerEl}></div>
    </div>
  </div>
  {#if selBtn}
    <button class="primary selbtn" style="left: {selBtn.x}px; top: {selBtn.y + 44}px" onmousedown={(e) => e.preventDefault()} onclick={addNoteFromSelection}>
      + Add note
    </button>
  {/if}
</div>

<style>
  .pdf { position: relative; height: 100%; display: flex; flex-direction: column; background: var(--viewer-bg); }
  .tools { display: flex; align-items: center; gap: 4px; padding: 4px 8px; border-bottom: 1px solid var(--border); background: var(--bg-elev); flex-wrap: wrap; }
  .pageform { display: flex; align-items: center; gap: 6px; }
  .pageform input { width: 4.5em; text-align: center; padding: 3px 6px; }
  .sep { width: 1px; height: 20px; background: var(--border); margin: 0 4px; }
  .zoom { min-width: 56px; justify-content: center; }
  .search { display: flex; align-items: center; gap: 4px; }
  .search input { width: 180px; padding: 3px 8px; }
  .small { font-size: 0.8rem; }
  .stage { position: relative; flex: 1; min-height: 0; }
  .container { position: absolute; inset: 0; overflow: auto; outline: none; }
  .selbtn { position: absolute; z-index: 20; box-shadow: var(--shadow); }
  :global(.pdfViewer .page) { box-shadow: var(--shadow); }
  :global(.bm-highlights) { position: absolute; inset: 0; pointer-events: none; z-index: 1; }
  :global(.bm-hl) { position: absolute; border-radius: 2px; mix-blend-mode: multiply; }
  :global(.bm-hl-yellow) { background: rgb(255 214 0 / 0.45); }
  :global(.bm-hl-green) { background: rgb(90 200 120 / 0.45); }
  :global(.bm-hl-blue) { background: rgb(80 150 255 / 0.4); }
  :global(.bm-hl-pink) { background: rgb(255 100 170 / 0.4); }
  :global(.bm-hl.flash) { animation: bm-flash 1.4s ease-out; }
  @keyframes -global-bm-flash { 0%, 40% { outline: 3px solid var(--accent); outline-offset: 2px; } 100% { outline: 3px solid transparent; } }
</style>
