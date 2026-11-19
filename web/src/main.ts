import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { flushQueue } from './lib/progress';

mount(App, { target: document.getElementById('app')! });

// Saves that failed earlier (offline, server down) are retried on load and when back online.
flushQueue();
window.addEventListener('online', () => flushQueue());
