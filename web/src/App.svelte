<script lang="ts">
  import { onMount } from 'svelte';
  import { api, setUnauthorizedHandler } from './lib/api';
  import { router } from './lib/router.svelte';
  import { session } from './lib/session.svelte';
  import { connection } from './lib/offline.svelte';
  import { notesSync, setNotesUser } from './lib/notes.svelte';
  import Settings from './pages/Settings.svelte';
  import Header from './components/Header.svelte';
  import Login from './pages/Login.svelte';
  import Home from './pages/Home.svelte';
  import LibraryPage from './pages/Library.svelte';
  import SeriesPage from './pages/Series.svelte';
  import BookDetail from './pages/BookDetail.svelte';
  import Admin from './pages/Admin.svelte';
  import Reader from './reader/Reader.svelte';

  let loadError = $state('');

  async function load() {
    loadError = '';
    try {
      const st = await api.status();
      session.setupRequired = st.setup_required;
      if (!st.setup_required) {
        try { session.user = await api.me(); } catch { session.user = null; }
      }
    } catch (e: any) {
      loadError = e.message;
    }
    session.ready = true;
  }

  onMount(() => {
    setUnauthorizedHandler(() => { session.user = null; });
    load();
  });

  const reader = $derived(router.match('/read/:id'));

  // Notes written offline are sent with the session of the user who wrote them.
  $effect(() => { setNotesUser(session.user?.id ?? null); });
</script>

{#if connection.offline || notesSync.pending || notesSync.failed}
  <a class="offline" aria-live="polite" href="/settings"
    title="Progress and notes are kept on this device and sent when the server is back. Details in Settings.">
    {#if connection.offline}Server unreachable · offline copy{/if}
    {#if notesSync.pending}{connection.offline ? ' · ' : ''}{notesSync.pending} {notesSync.pending === 1 ? 'note' : 'notes'} waiting to sync{/if}
    {#if notesSync.failed}<span class="error"> · {notesSync.failed} not synced</span>{/if}
  </a>
{/if}

{#if !session.ready}
  <div class="empty">Loading…</div>
{:else if loadError}
  <div class="empty">
    <h2>Can't reach the server</h2>
    <p>{loadError}</p>
    <button onclick={load}>Retry</button>
  </div>
{:else if !session.user}
  <Login setup={session.setupRequired} />
{:else if reader}
  {#key reader.id}
    <Reader id={Number(reader.id)} />
  {/key}
{:else}
  <Header />
  <main>
    {#if router.match('/')}
      <Home />
    {:else if router.match('/library/:id')}
      {@const p = router.match('/library/:id')!}
      {#key p.id}<LibraryPage id={Number(p.id)} />{/key}
    {:else if router.match('/series/:id')}
      {@const p = router.match('/series/:id')!}
      {#key p.id}<SeriesPage id={Number(p.id)} />{/key}
    {:else if router.match('/book/:id')}
      {@const p = router.match('/book/:id')!}
      {#key p.id}<BookDetail id={Number(p.id)} />{/key}
    {:else if router.match('/settings')}
      <Settings />
    {:else if router.match('/admin') && session.user.is_admin}
      <Admin />
    {:else}
      <div class="empty"><h2>Not found</h2><a href="/">Back to the library</a></div>
    {/if}
  </main>
{/if}

<style>
  .offline {
    position: fixed; left: 12px; bottom: 12px; z-index: 200;
    padding: 4px 10px; border-radius: 999px; font-size: 12px;
    background: var(--bg-elev); color: var(--fg-muted); border: 1px solid var(--border);
    box-shadow: 0 2px 8px rgb(0 0 0 / 0.15); text-decoration: none;
  }
</style>
