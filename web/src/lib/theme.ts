export type Theme = 'system' | 'light' | 'dark';
const KEY = 'bookman:theme';

export function getTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY);
    if (t === 'light' || t === 'dark') return t;
  } catch {}
  return 'system';
}

export function setTheme(t: Theme) {
  try {
    if (t === 'system') localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, t);
  } catch {}
  if (t === 'system') delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = t;
}

export function isDark(): boolean {
  const t = getTheme();
  if (t !== 'system') return t === 'dark';
  return matchMedia('(prefers-color-scheme: dark)').matches;
}
