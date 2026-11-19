import { test, expect, type Page, type BrowserContext } from '@playwright/test';

// Needs a user and a PDF in the library with at least 25 pages; the fixture
// e2e/fixtures/test-book.pdf ("Bookman Test Book", 30 pages) is made for this.
const USER = process.env.BOOKMAN_E2E_USER ?? 'admin';
const PASSWORD = process.env.BOOKMAN_E2E_PASSWORD ?? 'password123';
const TITLE = process.env.BOOKMAN_E2E_BOOK ?? 'Bookman Test Book';

async function login(context: BrowserContext): Promise<Page> {
  const page = await context.newPage();
  const res = await page.request.post('/api/auth/login', { data: { username: USER, password: PASSWORD } });
  expect(res.ok()).toBeTruthy();
  return page;
}

async function findBook(page: Page): Promise<number> {
  const libs = await (await page.request.get('/api/libraries')).json();
  for (const l of libs) {
    const r = await (await page.request.get(`/api/libraries/${l.id}/books?q=${encodeURIComponent(TITLE)}`)).json();
    if (r.books.length) return r.books[0].id;
  }
  throw new Error(`book "${TITLE}" not found`);
}

/** Wait until the viewer has rendered `n` (the page box shows the target before the PDF is loaded). */
async function rendered(page: Page, n: number) {
  await expect(page.locator(`.pdf .page[data-page-number="${n}"] canvas`)).toBeVisible({ timeout: 20_000 });
}

async function pageNumber(page: Page): Promise<number> {
  return Number(await page.getByLabel('Page', { exact: true }).inputValue());
}

/** Scroll the PDF so the top of the viewport is `offset` into `pageNo`. */
async function scrollTo(page: Page, pageNo: number, offset: number) {
  await page.evaluate(([n, o]) => {
    const c = document.querySelector('.pdf .container') as HTMLElement;
    const p = c.querySelector(`.page[data-page-number="${n}"]`) as HTMLElement;
    c.dispatchEvent(new WheelEvent('wheel'));
    c.scrollTop = p.offsetTop + o * p.offsetHeight;
  }, [pageNo, offset]);
  await expect.poll(() => pageNumber(page)).toBe(pageNo);
}

test('closing the tab mid-book and reopening resumes at the same spot', async ({ browser }) => {
  const ctx = await browser.newContext();
  let page = await login(ctx);
  const id = await findBook(page);
  await page.request.delete(`/api/books/${id}/progress`);

  await page.goto(`/read/${id}`);
  await rendered(page, 1);
  await scrollTo(page, 12, 0.5);

  // Close without waiting for the debounce: pagehide must flush with sendBeacon.
  await page.close({ runBeforeUnload: true });

  await expect.poll(async () => {
    const p = await (await ctx.request.get(`/api/books/${id}/progress`)).json();
    return p?.page;
  }).toBe(12);

  page = await ctx.newPage();
  await page.goto(`/read/${id}`);
  await rendered(page, 12);
  await expect.poll(() => pageNumber(page)).toBe(12);
  // The restored offset is within the page, not at its top.
  const offset = await page.evaluate(() => {
    const c = document.querySelector('.pdf .container') as HTMLElement;
    const p = c.querySelector('.page[data-page-number="12"]') as HTMLElement;
    return (c.scrollTop - p.offsetTop) / p.offsetHeight;
  });
  expect(offset).toBeGreaterThan(0.4);
  expect(offset).toBeLessThan(0.6);
  await ctx.close();
});

test('another device further ahead is offered with a prompt', async ({ browser }) => {
  const a = await browser.newContext();
  const b = await browser.newContext();
  const pa = await login(a);
  const id = await findBook(pa);
  await pa.request.delete(`/api/books/${id}/progress`);

  // Device A reads to page 5.
  await pa.goto(`/read/${id}`);
  await rendered(pa, 1);
  await scrollTo(pa, 5, 0.1);
  await pa.close({ runBeforeUnload: true });

  // Device B opens at A's position (no local history) and reads on to page 20.
  const pb = await login(b);
  await pb.goto(`/read/${id}`);
  await rendered(pb, 5);
  await expect.poll(() => pageNumber(pb)).toBe(5);
  await scrollTo(pb, 20, 0.2);
  await pb.close({ runBeforeUnload: true });
  await expect.poll(async () => (await (await b.request.get(`/api/books/${id}/progress`)).json())?.page).toBe(20);

  // Back on A: it opens where A left off and offers B's page.
  const pa2 = await a.newPage();
  await pa2.goto(`/read/${id}`);
  await rendered(pa2, 5);
  await expect.poll(() => pageNumber(pa2)).toBe(5);
  const prompt = pa2.getByRole('status').filter({ hasText: 'You were on page 20' });
  await expect(prompt).toBeVisible();
  await prompt.getByRole('button', { name: 'Jump' }).click();
  await expect.poll(() => pageNumber(pa2)).toBe(20);
  await a.close();
  await b.close();
});
