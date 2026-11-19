export function percent(p: number | undefined | null): string {
  return `${Math.round((p ?? 0) * 100)}%`;
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB'];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
}

export function dateTime(ms: number): string {
  return new Date(ms).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

export function ago(ms: number): string {
  const s = Math.round((Date.now() - ms) / 1000);
  if (s < 60) return 'just now';
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d} d ago`;
  return new Date(ms).toLocaleDateString();
}

export function duration(ms: number): string {
  const m = Math.max(1, Math.round(ms / 60000));
  if (m < 60) return `${m} min`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

const SMALL = new Set(['a', 'an', 'and', 'as', 'at', 'by', 'for', 'in', 'of', 'on', 'or', 'the', 'to', 'vs']);

/** "finance_and_accounting" -> "Finance and Accounting". Names with capitals are kept as typed. */
export function humanize(name: string): string {
  const spaced = name.replace(/_+/g, ' ').replace(/\s+/g, ' ').trim();
  if (spaced !== spaced.toLowerCase()) return spaced;
  return spaced
    .split(' ')
    .map((w, i) => (i > 0 && SMALL.has(w) ? w : w.replace(/(^|-)(\p{L})/gu, (_, d, c) => d + c.toUpperCase())))
    .join(' ');
}
