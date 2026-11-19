import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// `make dev` runs the Go server on :8080 and this dev server on :5173.
export default defineConfig({
  plugins: [svelte()],
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 2000, target: 'es2022' },
  server: {
    proxy: { '/api': { target: process.env.BOOKMAN_API ?? 'http://localhost:8080', changeOrigin: false } },
  },
});
