import { test, expect } from '@playwright/test';

const USER = process.env.BOOKMAN_E2E_USER ?? 'admin';
const PASSWORD = process.env.BOOKMAN_E2E_PASSWORD ?? 'password123';
const TITLE = process.env.BOOKMAN_E2E_BOOK ?? 'Bookman Test Book';

// The service worker cuts the server off for the page too: context.setOffline
// also stops the worker's own requests, like the server going down.
test('read and write notes with the server unreachable; they sync when it is back', async ({ page, context }) => {
  expect((await page.request.post('/api/auth/login', { data: { username: USER, password: PASSWORD } })).ok()).toBeTruthy();
  let id = 0;
  for (const l of await (await page.request.get('/api/libraries')).json()) {
    const r = await (await page.request.get(`/api/libraries/${l.id}/books?q=${encodeURIComponent(TITLE)}`)).json();
    if (r.books.length) id = r.books[0].id;
  }
  for (const n of await (await page.request.get(`/api/books/${id}/notes`)).json()) await page.request.delete(`/api/notes/${n.id}`);
  await page.request.delete(`/api/books/${id}/progress`);

  // First visit installs the worker; from then on it controls the page.
  await page.goto('/');
  await page.waitForFunction(() => !!navigator.serviceWorker?.controller, null, { timeout: 15_000 });

  // Opening the book saves it for offline reading in the background.
  await page.goto(`/read/${id}`);
  await expect(page.locator('.pdf .page[data-page-number="1"] .textLayer span').first()).toBeVisible({ timeout: 20_000 });
  await expect.poll(() => page.evaluate(async (id) => !!(await (await caches.open('bookman-books')).match(`/offline/books/${id}/meta`)), id), { timeout: 30_000 })
    .toBe(true);

  await context.setOffline(true);
  await page.reload();
  await expect(page.locator('.pdf .page[data-page-number="1"] .textLayer span').first()).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('a.offline')).toContainText('Server unreachable');

  // Pages further in come from the saved copy too.
  await page.getByRole('textbox', { name: 'Page' }).fill('20');
  await page.getByRole('textbox', { name: 'Page' }).press('Enter');
  await expect(page.locator('.pdf .page[data-page-number="20"] .textLayer span').first()).toBeVisible({ timeout: 20_000 });

  // A note written now is kept on the device and survives a reload.
  await page.getByRole('button', { name: /\+ Note on page/ }).click();
  await page.locator('article.note textarea').first().fill('Written with the server down.');
  await expect(page.getByText('Saved on this device')).toBeVisible({ timeout: 5000 });
  await expect(page.locator('a.offline')).toContainText('1 note waiting to sync');
  await page.reload();
  await page.getByRole('tab', { name: /All/ }).click();
  await expect(page.locator('article.note')).toContainText('Written with the server down.');

  // Back online: the note reaches the server and the indicator goes away.
  await context.setOffline(false);
  await expect.poll(async () => (await (await page.request.get(`/api/books/${id}/notes`)).json()).map((n: any) => n.body_md), { timeout: 30_000 })
    .toEqual(['Written with the server down.']);
  await expect(page.locator('a.offline')).toHaveCount(0, { timeout: 15_000 });

  // Clean up for the other tests.
  for (const n of await (await page.request.get(`/api/books/${id}/notes`)).json()) await page.request.delete(`/api/notes/${n.id}`);
});
