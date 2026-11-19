<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { dateTime } from '../lib/format';
  import { refreshLibraries } from '../lib/libraries.svelte';
  import type { AdminLibrary, User } from '../lib/types';
  import LibraryEditor from '../components/LibraryEditor.svelte';

  let libs = $state<AdminLibrary[]>([]);
  let mediaRoots = $state<string[]>([]);
  let editing = $state<AdminLibrary | 'new' | null>(null);
  let notice = $state('');

  async function loadLibraries() {
    try {
      const r = await api.adminLibraries();
      libs = r.libraries; mediaRoots = r.media_roots;
    } catch (e: any) { error = e.message; }
  }
  async function saved(l: AdminLibrary) {
    notice = `${editing === 'new' ? 'Added' : 'Saved'} ${l.name}; scanning it now.`;
    editing = null;
    await loadLibraries();
    refreshLibraries().catch(() => {});
    setTimeout(refresh, 300);
  }
  async function removeLib(l: AdminLibrary) {
    if (!confirm(`Remove the library "${l.name}"?\n\nIts books disappear from bookman. Files on disk are not touched, and reading progress and notes are kept: adding the same folder again brings everything back.`)) return;
    try {
      await api.removeLibrary(l.id);
      notice = `Removed ${l.name}.`;
      await loadLibraries();
      refreshLibraries().catch(() => {});
    } catch (e: any) { error = e.message; }
  }
  async function scanLib(l: AdminLibrary) {
    try { await api.scanLibrary(l.id); setTimeout(refresh, 300); } catch (e: any) { error = e.message; }
  }

  let users = $state<User[]>([]);
  let scan = $state<any>(null);
  let coversPending = $state(0);
  let username = $state('');
  let password = $state('');
  let isAdmin = $state(false);
  let message = $state('');
  let error = $state('');

  async function refresh() {
    try {
      const s = await api.scanStatus();
      scan = s.scan; coversPending = s.covers_pending;
    } catch {}
  }
  onMount(() => {
    loadLibraries();
    api.users().then((u) => (users = u)).catch((e) => (error = e.message));
    refresh();
    const t = setInterval(refresh, 3000);
    return () => clearInterval(t);
  });

  async function startScan() {
    await api.scan().catch((e) => (error = e.message));
    setTimeout(refresh, 300);
  }

  async function addUser(e: SubmitEvent) {
    e.preventDefault();
    error = ''; message = '';
    try {
      const u = await api.createUser(username, password, isAdmin);
      users = [...users, u];
      message = `Created ${u.username}.`;
      username = ''; password = ''; isAdmin = false;
    } catch (err: any) { error = err.message; }
  }
</script>

<div class="page">
  <h1>Admin</h1>
  {#if error}<p class="error">{error}</p>{/if}
  <section class="card">
    <div class="row">
      <h2>Libraries</h2>
      <div class="spacer"></div>
      {#if editing !== 'new'}<button class="primary" onclick={() => (editing = 'new')}>+ Add library</button>{/if}
    </div>
    {#if notice}<p class="muted">{notice}</p>{/if}
    {#if editing === 'new'}
      <LibraryEditor onsaved={saved} oncancel={() => (editing = null)} />
    {/if}
    {#if libs.length === 0 && editing !== 'new'}
      <p class="muted">No libraries yet. Add a folder of PDFs and comics to get started.</p>
    {/if}
    <div class="libs">
      {#each libs as l (l.id)}
        {#if editing !== 'new' && editing?.id === l.id}
          <LibraryEditor library={l} onsaved={saved} oncancel={() => (editing = null)} />
        {:else}
          <div class="lib">
            <div class="info">
              <a class="name" href="/library/{l.id}">{l.name}</a>
              <code class="path">{l.path}</code>
              <span class="faint small">
                {l.book_count} {l.book_count === 1 ? 'book' : 'books'}
                {#if l.exclude.length} · skips {l.exclude.join(', ')}{/if}
              </span>
              {#if !l.exists}<span class="error small">The folder is missing or can't be read.</span>{/if}
            </div>
            <div class="actions">
              <button onclick={() => scanLib(l)} disabled={scan?.running}>Scan</button>
              <button onclick={() => (editing = l)}>Edit</button>
              <button class="danger" onclick={() => removeLib(l)}>Remove</button>
            </div>
          </div>
        {/if}
      {/each}
    </div>
    <p class="faint small">Folders you can add from: {mediaRoots.join(', ')} (<code>media_roots</code> in the config file).</p>
  </section>

  <section class="card">
    <div class="row"><h2>Library scan</h2><div class="spacer"></div><button onclick={startScan} disabled={scan?.running}>{scan?.running ? 'Scanning…' : 'Scan all libraries'}</button></div>
    {#if scan}
      <p class="muted">
        {#if scan.running}Scanning {scan.library || 'all libraries'} since {dateTime(scan.started_at)}.{:else if scan.finished_at}Last scan finished {dateTime(scan.finished_at)}.{/if}
        {scan.seen} files seen, {scan.added} added, {scan.moved} moved, {scan.missing} missing, {scan.errors} errors.
        {coversPending} covers waiting.
      </p>
      {#if scan.last_error}<p class="faint small">Last error: {scan.last_error}</p>{/if}
    {/if}
  </section>

  <section class="card">
    <h2>Users</h2>
    <table>
      <thead><tr><th>Username</th><th>Role</th><th>Created</th></tr></thead>
      <tbody>{#each users as u (u.id)}<tr><td>{u.username}</td><td>{u.is_admin ? 'Admin' : 'Reader'}</td><td>{dateTime(u.created_at)}</td></tr>{/each}</tbody>
    </table>
    <form class="row" onsubmit={addUser}>
      <input placeholder="Username" bind:value={username} required autocomplete="off" />
      <input placeholder="Password (8+ characters)" type="password" bind:value={password} required minlength="8" autocomplete="new-password" />
      <label class="row"><input type="checkbox" bind:checked={isAdmin} /> Admin</label>
      <button class="primary">Add user</button>
    </form>
    {#if message}<p class="muted">{message}</p>{/if}
  </section>
</div>

<style>
  section { margin-bottom: 20px; }
  table { border-collapse: collapse; width: 100%; margin-bottom: 16px; }
  th, td { text-align: left; padding: 6px 10px 6px 0; border-bottom: 1px solid var(--border); }
  th { color: var(--fg-faint); font-weight: 500; }
  .small { font-size: 0.85rem; }
  .libs { display: flex; flex-direction: column; gap: 10px; margin: 12px 0; }
  .lib { display: flex; align-items: center; gap: 12px; padding: 12px; border: 1px solid var(--border); border-radius: 8px; flex-wrap: wrap; }
  .info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1 1 260px; }
  .name { font-weight: 650; color: var(--fg); }
  .path { font-size: 0.82rem; color: var(--fg-muted); overflow-wrap: anywhere; }
  .actions { display: flex; gap: 6px; }
</style>
