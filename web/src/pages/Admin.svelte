<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { dateTime } from '../lib/format';
  import type { User } from '../lib/types';

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
    <div class="row"><h2>Library scan</h2><div class="spacer"></div><button onclick={startScan} disabled={scan?.running}>{scan?.running ? 'Scanning…' : 'Scan now'}</button></div>
    {#if scan}
      <p class="muted">
        {#if scan.running}Running since {dateTime(scan.started_at)}.{:else if scan.finished_at}Last scan finished {dateTime(scan.finished_at)}.{/if}
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
</style>
