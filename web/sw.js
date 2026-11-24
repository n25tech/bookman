// Bookman service worker: keeps the app and your books usable when the server
// is down. Built by the `serviceWorker` plugin in vite.config.ts, which fills in
// BUILD below with { version, files } (the hashed assets of this build).
//
//   app shell (navigations)       network first, cached index.html when offline
//   /assets/*                     precached at install, cache first (immutable)
//   API reads (JSON)              network first, last good copy when the server is unreachable
//   covers                        cache first (their URLs are immutable), at most MEDIA_MAX_ENTRIES
//   saved books                   src/lib/offline.svelte.ts saves a book when it is opened (PDFs in
//                                 1 MiB chunks, comics page by page); this answers PDF.js's range
//                                 requests from the chunks, reading only the ones asked for
//
// Writes (progress, notes, admin) are never cached; progress already queues
// itself in IndexedDB when the server is unreachable.

const BUILD = self.__BUILD__;
const SHELL = `bookman-shell-${BUILD.version}`;
const API = 'bookman-api';
const MEDIA = 'bookman-media';
const BOOKS = 'bookman-books';
const KEEP = new Set([SHELL, API, MEDIA, BOOKS]);

// Give up on a slow server after this long when there is a cached copy to show.
const NETWORK_TIMEOUT_MS = 6000;
// Covers kept, oldest dropped first (a few tens of KB each).
const MEDIA_MAX_ENTRIES = 2000;
// API answers kept (each search or folder is one).
const API_MAX_ENTRIES = 1000;

self.addEventListener('install', (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(SHELL);
    await cache.addAll(['/', '/favicon.svg', ...BUILD.files.map((f) => '/' + f)]);
    await self.skipWaiting();
  })());
});

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    for (const name of await caches.keys()) {
      if (name.startsWith('bookman-') && !KEEP.has(name)) await caches.delete(name);
    }
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;

  if (!url.pathname.startsWith('/api/')) {
    if (req.mode === 'navigate') return event.respondWith(shell(req));
    if (url.pathname.startsWith('/assets/') || url.pathname === '/favicon.svg') return event.respondWith(cacheFirst(SHELL, req));
    return;
  }
  // Opening an API URL itself (a file in a new tab, the notes export) goes to the server.
  if (req.mode === 'navigate') return;
  // saveBook() in src/lib/offline.svelte.ts downloads and stores books itself.
  if (req.headers.has('X-Bookman-Save')) return;

  let m;
  if ((m = /^\/api\/books\/(\d+)\/file$/.exec(url.pathname))) return event.respondWith(bookFile(req, m[1]));
  if (/^\/api\/books\/\d+\/pages\/\d+$/.test(url.pathname)) return event.respondWith(savedOrNetwork(req));
  if (/^\/api\/books\/\d+\/cover$/.test(url.pathname)) return event.respondWith(media(req, event));
  if (url.pathname.startsWith('/api/admin/') || url.pathname.endsWith('/export.md')) return;
  event.respondWith(apiRead(req, event));
});

// The server is "down" when the fetch fails or a proxy in front of it answers
// for it (502/503/504). Anything else, including 401 and 404, is a real answer.
const down = (res) => res.status === 502 || res.status === 503 || res.status === 504;

function withTimeout(promise, ms) {
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error('timeout')), ms);
    promise.then((v) => { clearTimeout(t); resolve(v); }, (e) => { clearTimeout(t); reject(e); });
  });
}

// Marks a response as served from the cache so the app can say it is offline.
function fromCache(res) {
  const headers = new Headers(res.headers);
  headers.set('X-Bookman-Offline', '1');
  return new Response(res.body, { status: res.status, statusText: res.statusText, headers });
}

async function shell(req) {
  const cache = await caches.open(SHELL);
  const cached = await cache.match('/');
  try {
    const net = fetch(req);
    const res = cached ? await withTimeout(net, NETWORK_TIMEOUT_MS) : await net;
    if (!down(res) || !cached) return res;
  } catch (e) {
    if (!cached) throw e;
  }
  return cached;
}

async function cacheFirst(name, req) {
  const cache = await caches.open(name);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) await cache.put(req, res.clone());
  return res;
}

async function apiRead(req, event) {
  const cache = await caches.open(API);
  const net = fetch(req).then(async (res) => {
    if (res.ok) {
      await cache.put(req, res.clone());
      await trim(cache, API_MAX_ENTRIES);
    }
    // Signed out or the item is gone: don't serve a stale copy later.
    else if (res.status === 401 || res.status === 404) await cache.delete(req);
    return res;
  });
  // If the cached copy is shown first, still let the fresh answer land in the cache.
  event.waitUntil(net.catch(() => {}));
  const cached = await cache.match(req);
  try {
    const res = cached ? await withTimeout(net, NETWORK_TIMEOUT_MS) : await net;
    if (!down(res) || !cached) return res;
  } catch (e) {
    if (!cached) throw e;
  }
  return fromCache(cached);
}

// Comic pages of saved books; others come from the server (and the HTTP cache).
async function savedOrNetwork(req) {
  return (await (await caches.open(BOOKS)).match(req)) ?? fetch(req);
}

async function media(req, event) {
  const cache = await caches.open(MEDIA);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) {
    await cache.put(req, res.clone());
    event.waitUntil(trim(cache, MEDIA_MAX_ENTRIES));
  }
  return res;
}

async function trim(cache, max) {
  // keys() is in insertion order (a put replaces and appends), so the oldest go first.
  const keys = await cache.keys();
  for (let i = 0; i < keys.length - max; i++) await cache.delete(keys[i]);
}

// A saved PDF answers PDF.js's requests, whole or by range, streaming only the
// chunks that cover the bytes asked for. Without one the request goes to the server.
async function bookFile(req, id) {
  const cache = await caches.open(BOOKS);
  const metaRes = await cache.match(`/offline/books/${id}/meta`);
  if (!metaRes) return fetch(req);
  const meta = await metaRes.json();
  const size = meta.size;
  let start = 0, end = size - 1, status = 200;
  const range = req.headers.get('Range');
  if (range) {
    const m = /^bytes=(\d*)-(\d*)$/.exec(range.trim());
    if (m && m[1] !== '') { start = Number(m[1]); if (m[2] !== '') end = Math.min(Number(m[2]), size - 1); }
    else if (m && m[2] !== '') start = Math.max(0, size - Number(m[2]));
    else start = NaN;
    if (!(start <= end) || start >= size) {
      return new Response(null, { status: 416, headers: { 'Content-Range': `bytes */${size}` } });
    }
    status = 206;
  }
  const first = Math.floor(start / meta.chunk), last = Math.floor(end / meta.chunk);
  let i = first;
  const body = new ReadableStream({
    async pull(ctrl) {
      if (i > last) return ctrl.close();
      const c = await cache.match(`/offline/books/${id}/${i}`);
      if (!c) return ctrl.error(new Error('saved book is incomplete'));
      let bytes = new Uint8Array(await c.arrayBuffer());
      const base = i * meta.chunk;
      bytes = bytes.subarray(Math.max(0, start - base), Math.min(bytes.length, end - base + 1));
      ctrl.enqueue(bytes);
      i++;
    },
  });
  const headers = new Headers({
    'Content-Type': meta.type || 'application/pdf',
    'Content-Length': String(end - start + 1),
    'Accept-Ranges': 'bytes',
    'Cache-Control': 'no-store',
  });
  if (status === 206) headers.set('Content-Range', `bytes ${start}-${end}/${size}`);
  return new Response(body, { status, statusText: status === 206 ? 'Partial Content' : 'OK', headers });
}
