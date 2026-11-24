import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Emits dist/sw.js from sw.js with the list of this build's files to precache,
// and a version that changes with them (so a new deploy replaces the old cache).
function serviceWorker(): Plugin {
  return {
    name: 'bookman-service-worker',
    apply: 'build',
    generateBundle(_, bundle) {
      const src = readFileSync(new URL('./sw.js', import.meta.url), 'utf8');
      const files = Object.keys(bundle).filter((f) => f !== 'index.html' && !f.endsWith('.map')).sort();
      const version = createHash('sha256').update(src).update(files.join('\n')).digest('hex').slice(0, 12);
      this.emitFile({ type: 'asset', fileName: 'sw.js', source: src.replace('self.__BUILD__', JSON.stringify({ version, files })) });
    },
  };
}

// `make dev` runs the Go server on :8080 and this dev server on :5173.
export default defineConfig({
  plugins: [svelte(), serviceWorker()],
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 2000, target: 'es2022' },
  server: {
    proxy: { '/api': { target: process.env.BOOKMAN_API ?? 'http://localhost:8080', changeOrigin: false } },
  },
});
