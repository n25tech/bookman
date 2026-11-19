// A small history-API router. Routes are matched in order.
type Params = Record<string, string>;

class Router {
  path = $state(location.pathname);
  search = $state(location.search);

  constructor() {
    window.addEventListener('popstate', () => this.sync());
    // Intercept same-origin link clicks.
    document.addEventListener('click', (e) => {
      const a = (e.target as HTMLElement).closest('a');
      if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      if (a.target || a.hasAttribute('download') || a.origin !== location.origin) return;
      if (a.pathname.startsWith('/api/')) return;
      e.preventDefault();
      this.go(a.pathname + a.search);
    });
  }

  sync() { this.path = location.pathname; this.search = location.search; }

  go(url: string, replace = false) {
    if (replace) history.replaceState(null, '', url);
    else history.pushState(null, '', url);
    this.sync();
    window.scrollTo(0, 0);
  }

  match(pattern: string): Params | null {
    const p = pattern.split('/'), u = this.path.split('/');
    if (p.length !== u.length) return null;
    const out: Params = {};
    for (let i = 0; i < p.length; i++) {
      if (p[i].startsWith(':')) out[p[i].slice(1)] = decodeURIComponent(u[i]);
      else if (p[i] !== u[i]) return null;
    }
    return out;
  }

  query(): URLSearchParams { return new URLSearchParams(this.search); }
}

export const router = new Router();
