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

test('book notes stay on every page and link to pages', async ({ page }) => {
  expect((await page.request.post('/api/auth/login', { data: { username: USER, password: PASSWORD } })).ok()).toBeTruthy();
  let id = 0;
  for (const l of await (await page.request.get('/api/libraries')).json()) {
    const r = await (await page.request.get(`/api/libraries/${l.id}/books?q=${encodeURIComponent(TITLE)}`)).json();
    if (r.books.length) id = r.books[0].id;
  }
  for (const n of await (await page.request.get(`/api/books/${id}/notes`)).json()) await page.request.delete(`/api/notes/${n.id}`);
  await page.request.delete(`/api/books/${id}/progress`);

  const pageBox = () => page.getByLabel('Page', { exact: true });
  await page.goto(`/read/${id}`);
  await expect(page.locator('.pdf .page[data-page-number="1"] canvas')).toBeVisible({ timeout: 20_000 });

  await page.getByRole('button', { name: '+ Book note' }).click();
  const card = page.locator('.booknotes article.note').first();
  const textarea = card.locator('textarea');
  await expect(textarea).toBeFocused();
  await textarea.pressSequentially('Start here. ');

  // Read on to page 5 and link it from the same note.
  await pageBox().fill('5');
  await pageBox().press('Enter');
  await expect(pageBox()).toHaveValue('5');
  await card.getByRole('button', { name: '🔗 p. 5' }).click();
  await textarea.press('End');
  await textarea.pressSequentially(' Ending: [[p. 9]]');
  await expect(textarea).toHaveValue('Start here. [[p. 5]] Ending: [[p. 9]]');
  await expect(card.getByText('Saved')).toBeVisible({ timeout: 5000 });

  // In preview, page links jump; the book note is still shown on the new page.
  await card.getByRole('button', { name: 'Preview' }).click();
  await card.getByRole('link', { name: 'p. 9' }).click();
  await expect(pageBox()).toHaveValue('9');
  await expect(page.locator('.booknotes article.note')).toHaveCount(1);

  const md = await (await page.request.get(`/api/books/${id}/notes/export.md`)).text();
  expect(md).toContain('## Book notes\n\nStart here. p. 5 Ending: p. 9');
});
