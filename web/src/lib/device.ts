// A stable per-browser device id, readable in the "you were on page X on <device>" prompt.
const KEY = 'bookman:device';

function describe(): string {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Browser';
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad|iPod/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : 'unknown';
  return `${browser} on ${os}`;
}

let cached: string | null = null;

export function deviceId(): string {
  if (cached) return cached;
  try {
    const v = localStorage.getItem(KEY);
    if (v) return (cached = v);
  } catch {}
  const rand = Array.from(crypto.getRandomValues(new Uint8Array(3)), (b) => b.toString(16).padStart(2, '0')).join('');
  cached = `${describe()} · ${rand}`;
  try { localStorage.setItem(KEY, cached); } catch {}
  return cached;
}

/** The human part of a device id ("Firefox on Linux"). */
export function deviceLabel(id: string): string {
  return id.split(' · ')[0] || id;
}
