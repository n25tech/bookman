import { test, expect } from '@playwright/test';

// Needs a media root (BOOKMAN_MEDIA_ROOTS) with a folder that isn't a library yet,
// holding at least one book: BOOKMAN_E2E_EXTRA (default "Extra"). CI makes one.
const USER = process.env.BOOKMAN_E2E_USER ?? 'admin';
const PASSWORD = process.env.BOOKMAN_E2E_PASSWORD ?? 'password123';
const EXTRA = process.env.BOOKMAN_E2E_EXTRA ?? 'Extra';

test('add a library from the admin page with the folder picker, then remove it', async ({ page }) => {
  expect((await page.request.post('/api/auth/login', { data: { username: USER, password: PASSWORD } })).ok()).toBeTruthy();
  const { libraries, media_roots } = await (await page.request.get('/api/admin/libraries')).json();
  for (const l of libraries) if (l.name === EXTRA) await page.request.delete(`/api/admin/libraries/${l.id}`);

  await page.goto('/admin');
  await page.getByRole('button', { name: '+ Add library' }).click();
  // The picker starts at the media roots; open the first one, then the extra folder.
  await page.getByRole('button', { name: media_roots[0] }).click();
  await page.getByRole('button', { name: EXTRA, exact: true }).click();
  await expect(page.locator('.picker code.path')).toHaveText(`${media_roots[0]}/${EXTRA}`);
  // Browsing must not submit the form (an empty required Name would get focus).
  await expect(page.getByLabel('Name')).not.toBeFocused();
  await page.getByRole('button', { name: 'Use this folder' }).click();
  await expect(page.getByLabel('Name')).toHaveValue(EXTRA);
  await page.getByRole('button', { name: 'Add library' }).click();

  const card = page.locator('.lib', { hasText: `${media_roots[0]}/${EXTRA}` });
  await expect(card).toBeVisible();
  // Scanned right away; the header lists it without a reload.
  await expect.poll(async () => {
    const libs = await (await page.request.get('/api/libraries')).json();
    return libs.find((l: any) => l.name === EXTRA)?.book_count ?? 0;
  }, { timeout: 20_000 }).toBeGreaterThan(0);
  await expect(page.locator('header nav').getByRole('link', { name: EXTRA })).toBeVisible();

  page.once('dialog', (d) => d.accept());
  await card.getByRole('button', { name: 'Remove' }).click();
  await expect(card).toHaveCount(0);
  await expect(page.locator('header nav').getByRole('link', { name: EXTRA })).toHaveCount(0);
});
