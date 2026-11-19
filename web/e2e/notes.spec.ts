import { test, expect } from '@playwright/test';

const USER = process.env.BOOKMAN_E2E_USER ?? 'admin';
const PASSWORD = process.env.BOOKMAN_E2E_PASSWORD ?? 'password123';
const TITLE = process.env.BOOKMAN_E2E_BOOK ?? 'Bookman Test Book';

test('select text, add an anchored note, see the highlight after reload', async ({ page }) => {
  expect((await page.request.post('/api/auth/login', { data: { username: USER, password: PASSWORD } })).ok()).toBeTruthy();
  let id = 0;
  for (const l of await (await page.request.get('/api/libraries')).json()) {
    const r = await (await page.request.get(`/api/libraries/${l.id}/books?q=${encodeURIComponent(TITLE)}`)).json();
    if (r.books.length) id = r.books[0].id;
  }
  for (const n of await (await page.request.get(`/api/books/${id}/notes`)).json()) await page.request.delete(`/api/notes/${n.id}`);
  await page.request.delete(`/api/books/${id}/progress`);

  await page.goto(`/read/${id}`);
  const span = page.locator('.pdf .page[data-page-number="1"] .textLayer span', { hasText: 'Page 1 line 3:' });
  await expect(span).toBeVisible({ timeout: 20_000 });

  // Select "quick brown fox" inside line 3.
  await span.evaluate((el) => {
    const t = el.firstChild as Text;
    const i = t.data.indexOf('quick brown fox');
    const r = document.createRange();
    r.setStart(t, i);
    r.setEnd(t, i + 'quick brown fox'.length);
    const sel = getSelection()!;
    sel.removeAllRanges();
    sel.addRange(r);
  });
  await page.getByRole('button', { name: '+ Add note' }).click();

  const card = page.locator('article.note').first();
  await expect(card.locator('blockquote')).toHaveText('quick brown fox');
  await card.locator('textarea').fill('Remember the **fox**.');
  await expect(card.getByText('Saved')).toBeVisible({ timeout: 5000 });
  await expect(page.locator('.bm-hl')).toHaveCount(1);

  const notes = await (await page.request.get(`/api/books/${id}/notes`)).json();
  expect(notes).toHaveLength(1);
  expect(notes[0]).toMatchObject({ page: 1, quote: 'quick brown fox', body_md: 'Remember the **fox**.' });
  expect(notes[0].anchor_end - notes[0].anchor_start).toBe('quick brown fox'.length);

  // After a reload the highlight comes back from the stored offsets.
  await page.reload();
  await expect(page.locator('.pdf .page[data-page-number="1"] .bm-hl')).toHaveCount(1, { timeout: 20_000 });
  await page.getByRole('tab', { name: /All/ }).click();
  await expect(page.locator('article.note .md strong')).toHaveText('fox');

  // Export contains the quote and the note.
  const md = await (await page.request.get(`/api/books/${id}/notes/export.md`)).text();
  expect(md).toContain('> quick brown fox');
  expect(md).toContain('Remember the **fox**.');

  // Delete with undo.
  await page.getByRole('button', { name: 'Delete note' }).click();
  await expect(page.locator('article.note')).toHaveCount(0);
  await page.getByRole('button', { name: 'Undo' }).click();
  await expect(page.locator('article.note')).toHaveCount(1);
});
