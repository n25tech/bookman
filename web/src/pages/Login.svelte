<script lang="ts">
  import { api } from '../lib/api';
  import { session } from '../lib/session.svelte';
  import ThemeToggle from '../components/ThemeToggle.svelte';

  let { setup }: { setup: boolean } = $props();
  let username = $state('');
  let password = $state('');
  let confirm = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (setup && password !== confirm) { error = 'The passwords do not match.'; return; }
    busy = true;
    try {
      session.user = setup ? await api.setup(username, password) : await api.login(username, password);
      session.setupRequired = false;
    } catch (err: any) {
      error = err.message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="wrap">
  <div class="corner"><ThemeToggle /></div>
  <form class="card" onsubmit={submit}>
    <div class="brand"><img src="/favicon.svg" alt="" width="34" height="34" /> Bookman</div>
    {#if setup}
      <h1>Create the admin account</h1>
      <p class="muted">No users exist yet. This first account can add others and run library scans.</p>
    {:else}
      <h1>Sign in</h1>
    {/if}
    <label>Username <input bind:value={username} autocomplete="username" required autofocus /></label>
    <label>Password <input type="password" bind:value={password} autocomplete={setup ? 'new-password' : 'current-password'} required minlength={setup ? 8 : undefined} /></label>
    {#if setup}
      <label>Confirm password <input type="password" bind:value={confirm} autocomplete="new-password" required /></label>
    {/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button class="primary" disabled={busy}>{setup ? 'Create account' : 'Sign in'}</button>
  </form>
</div>

<style>
  .wrap { min-height: 100vh; display: grid; place-items: center; padding: 16px; }
  .corner { position: fixed; top: 10px; right: 10px; }
  form { width: min(380px, 100%); display: flex; flex-direction: column; gap: 14px; padding: 28px; }
  .brand { display: flex; align-items: center; gap: 10px; font-weight: 700; font-size: 1.3rem; margin-bottom: 8px; }
  h1 { font-size: 1.2rem; margin: 0; }
  p { margin: 0; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 0.9rem; color: var(--fg-muted); }
  button { justify-content: center; padding: 9px; }
</style>
