// Text anchors are character offsets into the concatenated text nodes of a
// PDF.js page text layer. The same walk is used to create and to render them.

function textNodes(root: Node): Text[] {
  const out: Text[] = [];
  const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) out.push(n as Text);
  return out;
}

/** Offsets of a DOM range within a text layer, or null if the range is outside it. */
export function rangeToOffsets(layer: HTMLElement, range: Range): { start: number; end: number } | null {
  if (!layer.contains(range.startContainer) || !layer.contains(range.endContainer)) return null;
  let pos = 0, start = -1, end = -1;
  for (const t of textNodes(layer)) {
    const len = t.data.length;
    if (start < 0) {
      if (t === range.startContainer) start = pos + range.startOffset;
      else if (range.startContainer.contains(t) || (range.startContainer.nodeType !== Node.TEXT_NODE && range.intersectsNode(t))) start = pos;
    }
    if (t === range.endContainer) { end = pos + range.endOffset; break; }
    if (range.intersectsNode(t)) end = pos + len;
    pos += len;
  }
  if (start < 0 || end <= start) return null;
  return { start, end };
}

/** A DOM range covering [start, end) of the text layer. */
export function offsetsToRange(layer: HTMLElement, start: number, end: number): Range | null {
  let pos = 0;
  const r = document.createRange();
  let startSet = false;
  for (const t of textNodes(layer)) {
    const len = t.data.length;
    if (!startSet && start < pos + len) { r.setStart(t, Math.max(0, start - pos)); startSet = true; }
    if (startSet && end <= pos + len) { r.setEnd(t, Math.max(0, end - pos)); return r; }
    pos += len;
  }
  return null;
}

/** Highlight rectangles as fractions of the page box, so they survive zoom. */
export function rangeRects(range: Range, page: HTMLElement): { x: number; y: number; w: number; h: number }[] {
  const pr = page.getBoundingClientRect();
  if (!pr.width || !pr.height) return [];
  return Array.from(range.getClientRects())
    .filter((r) => r.width > 0.5 && r.height > 0.5)
    .map((r) => ({ x: (r.left - pr.left) / pr.width, y: (r.top - pr.top) / pr.height, w: r.width / pr.width, h: r.height / pr.height }));
}
