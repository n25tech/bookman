import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { flushQueue } from './lib/progress';
import { registerServiceWorker } from './lib/offline.svelte';
import { flushNotes } from './lib/notes.svelte';

registerServiceWorker();
mount(App, { target: document.getElementById('app')! });

// Saves that failed earlier (offline, server down) are retried on load and when back online.
flushQueue();
window.addEventListener('online', () => { flushQueue(); flushNotes(); });
// Coming back to the tab is a good moment to retry too (phones drop timers in the background).
document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') { flushQueue(); flushNotes(); } });
