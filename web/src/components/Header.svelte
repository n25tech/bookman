<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { router } from '../lib/router.svelte';
  import { session } from '../lib/session.svelte';
  import type { Library } from '../lib/types';
  import ThemeToggle from './ThemeToggle.svelte';

  let libraries = $state<Library[]>([]);
  let menuOpen = $state(false);
  onMount(async () => { try { libraries = await api.libraries(); } catch {} });

  async function logout() {
    await api.logout().catch(() => {});
    session.user = null;
    router.go('/', true);
  }
</script>

<header>
  <a class="brand" href="/">
    <img src="/favicon.svg" alt="" width="22" height="22" /> Bookman
  </a>
  <nav class:open={menuOpen}>
    {#each libraries as l (l.id)}
      <a href="/library/{l.id}" class:active={router.path === `/library/${l.id}`} onclick={() => (menuOpen = false)}>{l.name}</a>
    {/each}
  </nav>
  <div class="spacer"></div>
  <ThemeToggle />
  {#if session.user?.is_admin}
    <a class="btn ghost" href="/admin" title="Admin">Admin</a>
  {/if}
  <button class="ghost" onclick={logout} title="Sign out {session.user?.username}">Sign out</button>
  <button class="ghost icon menu" aria-label="Libraries" onclick={() => (menuOpen = !menuOpen)}>☰</button>
</header>

<style>
  header {
    position: sticky; top: 0; z-index: 50;
    height: var(--header-h);
    display: flex; align-items: center; gap: 6px;
    padding: 0 12px;
    background: color-mix(in srgb, var(--bg-elev) 92%, transparent);
    backdrop-filter: blur(8px);
    border-bottom: 1px solid var(--border);
  }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 650; color: var(--fg); margin-right: 12px; }
  .brand:hover { text-decoration: none; }
  nav { display: flex; gap: 2px; overflow-x: auto; }
  nav a { padding: 6px 10px; border-radius: 6px; color: var(--fg-muted); white-space: nowrap; }
  nav a:hover { background: var(--bg-sunken); text-decoration: none; color: var(--fg); }
  nav a.active { color: var(--fg); background: var(--bg-sunken); }
  .menu { display: none; }
  @media (max-width: 700px) {
    .menu { display: inline-flex; }
    nav {
      display: none; position: absolute; top: var(--header-h); left: 0; right: 0;
      flex-direction: column; background: var(--bg-elev); border-bottom: 1px solid var(--border); padding: 8px;
    }
    nav.open { display: flex; }
  }
</style>
