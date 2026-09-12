import { test, expect, type BrowserContext, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

async function post(context: BrowserContext, path: string, data: unknown) {
  const response = await context.request.post('/api/v1' + path, { headers: { 'X-RoomDeck-Request': '1' }, data });
  expect(response.ok(), await response.text()).toBeTruthy();
  return response.json();
}
async function fixture(context: BrowserContext) {
  await post(context, '/login', { username: 'e2e-host', password: 'test-only-strong-password-2026' });
  return post(context, '/rooms', { name: '周末相聚 A room with a longer name', duration: 7200, retention: 86400 });
}
async function noOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const overflow = await page.locator('button, input:not([hidden]), select, textarea').evaluateAll(elements => elements.filter(el => {
    const box = el.getBoundingClientRect();
    return box.width && box.height && (box.right > innerWidth + 1 || box.left < -1) && getComputedStyle(el).visibility !== 'hidden';
  }).map(el => el.outerHTML.slice(0, 150)));
  expect(overflow).toEqual([]);
}
async function accessible(page: Page) {
  // Assess the settled interface, not a transient opacity during its entrance.
  await page.evaluate(() => Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))));
  const report = await new AxeBuilder({ page }).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  expect(report.violations.map(v => ({ id: v.id, nodes: v.nodes.map(n => ({ target: n.target, summary: n.failureSummary })) }))).toEqual([]);
}

test('responsive bilingual UI: routes, navigation, alignment and dialog accessibility', async ({ browser, context, page, baseURL }) => {
  test.setTimeout(120000);
  const room = await fixture(context);
  const guest = await browser.newContext({ baseURL });
  try {
    await post(guest, '/join', { code: room.code, name: 'A guest with a name' });
    const gp = await guest.newPage();
    const anon = await browser.newContext({ baseURL });
    try {
      const ap = await anon.newPage();
      const loginPage = await anon.newPage();
      await ap.goto('/join?code=' + room.code);
      await loginPage.goto('/login');
      for (const locale of ['zh','en']) {
        for (const width of [320,390,768,1280,1440]) {
          await page.setViewportSize({ width, height: 900 });
          await page.goto('/room/' + room.id);
          await page.locator('#language').selectOption(locale);
          await expect(page.locator('.feed')).toBeVisible();
          await noOverflow(page);
          await gp.setViewportSize({ width, height: 900 });
          await gp.goto('/room/' + room.id);
          await gp.locator('#language').selectOption(locale);
          await expect(gp.locator('.feed')).toBeVisible();
          await noOverflow(gp);
          await expect(gp.locator('.guest-bottom')).toBeVisible({ visible: width <= 640 });
          const nav = gp.locator(width <= 640 ? '.guest-bottom' : '.tabs');
          await nav.getByRole('button', { name: locale === 'zh' ? '游戏' : 'Games', exact: true }).click();
          await expect(gp.locator('.game-library')).toBeVisible();
          await noOverflow(gp);
          await nav.getByRole('button', { name: locale === 'zh' ? '投屏' : 'Screen sharing', exact: true }).click();
          await expect(gp.locator('.screen-empty')).toBeVisible();
          await noOverflow(gp);
          for (const publicPage of [loginPage, ap]) {
            await publicPage.setViewportSize({ width, height: 900 });
            await publicPage.locator('#language').selectOption(locale);
            await expect(publicPage.locator('form')).toBeVisible();
            await noOverflow(publicPage);
          }
        }
      }
      await accessible(ap);
      await accessible(loginPage);
    } finally { await anon.close(); }
    await page.goto('/');
    await page.locator('.room-card').first().waitFor();
    await accessible(page);
    await page.goto('/room/' + room.id);
    await page.locator('#language').selectOption('en');
    await expect(page.locator('.feed')).toBeVisible();
    await accessible(page);
    await page.getByRole('button', { name: 'Settings', exact: true }).last().click();
    await expect(page.getByRole('dialog', { name: 'Room settings', exact: true })).toBeVisible();
    await accessible(page);
    for (const width of [320,390,768,1440]) {
      await page.setViewportSize({ width, height: 844 });
      await noOverflow(page);
      await page.getByRole('dialog').evaluate(el => el.scrollTop = el.scrollHeight);
      const header = await page.locator('.modal-header').boundingBox();
      const dialog = await page.getByRole('dialog').boundingBox();
      expect(Math.abs(header!.y - dialog!.y)).toBeLessThan(3);
      await expect(page.getByRole('button', { name: 'Close', exact: true })).toBeVisible();
    }
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Settings', exact: true }).last()).toBeFocused();
    await page.locator('.tabs').getByRole('button', { name: 'Games', exact: true }).click();
    await page.locator('.game-library').waitFor();
    await accessible(page);
    await page.locator('.tabs').getByRole('button', { name: 'Screen sharing', exact: true }).click();
    await page.locator('.screen-empty').waitFor();
    await accessible(page);
  } finally { await guest.close(); }
});

test('share dialog: upload retry continues after dismissal and settings errors remain visible', async ({ context, page }) => {
  const room = await fixture(context);
  await page.goto('/room/' + room.id);
  await page.locator('#language').selectOption('en');
  await page.getByRole('button', { name: 'Share something', exact: true }).first().click();
  await expect(page.locator('input[type=file]')).toBeHidden();
  await page.getByRole('dialog').getByRole('button', { name: 'Files', exact: true }).click();
  let release!: () => void;
  let attempts = 0;
  await page.route('**/uploads/*/parts/*', async route => {
    attempts++;
    if (attempts === 1) return route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL_ERROR' } }) });
    await new Promise<void>(resolve => release = resolve);
    await route.continue();
  });
  await page.locator('input[type=file]').setInputFiles({ name: 'hello.txt', mimeType: 'text/plain', buffer: Buffer.from('RoomDeck UI regression') });
  await expect(page.getByRole('button', { name: 'Try again', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Try again', exact: true }).click();
  await expect(page.getByRole('dialog').getByRole('button', { name: 'Photos', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Close', exact: true })).toBeEnabled();
  await expect.poll(() => !!release).toBe(true);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  release();
  await expect(page.locator('.content-card.file')).toHaveCount(1);
  const download = await page.locator('.content-card.file a.icon-button').boundingBox();
  expect(download!.width).toBeGreaterThanOrEqual(44);
  expect(download!.height).toBeGreaterThanOrEqual(44);
  await page.getByRole('button', { name: 'Settings', exact: true }).last().click();
  await page.route('**/api/v1/rooms/' + room.id, route => route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: { code: 'VERSION_CONFLICT' } }) }));
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes', exact: true }).click();
  await expect(page.getByRole('dialog').getByRole('status')).toBeVisible();
  await expect(page.locator('body > .toast-message')).toHaveCount(0);
});

test('game choices survive sync, need confirmation, and private words conceal on navigation', async ({ context, page, browser, baseURL }) => {
  const room = await fixture(context);
  const guests: BrowserContext[] = [];
  try {
    for (let i = 0; i < 3; i++) {
      const guest = await browser.newContext({ baseURL }); guests.push(guest);
      await post(guest, '/join', { code: room.code, name: 'Guest ' + i });
    }
    const path = '/rooms/' + room.id + '/game';
    let game = await post(context, path, { action: 'create', kind: 'dice', language: 'en', version: 0 });
    for (const c of [context, guests[0]]) game = await post(c, path, { action: 'join', version: game.version });
    game = await post(context, path, { action: 'start', version: game.version });
    await page.goto('/room/' + room.id);
    await page.locator('#language').selectOption('en');
    await page.locator('.tabs').getByRole('button', { name: 'Games', exact: true }).click();
    await page.getByRole('button', { name: 'Guess 3', exact: true }).click();
    const synced = page.waitForResponse(r => r.url().endsWith(path) && r.request().method() === 'GET');
    await synced;
    await expect(page.getByRole('button', { name: 'Guess 3', exact: true })).toHaveAttribute('aria-pressed', 'true');
    let state = await (await context.request.get('/api/v1' + path)).json();
    expect(state.mine.guess).toBeFalsy();
    await accessible(page);
    await page.getByRole('button', { name: 'Confirm choice', exact: true }).click();
    await expect.poll(async () => (await (await context.request.get('/api/v1' + path)).json()).mine.guess).toBe(3);
    state = await (await context.request.get('/api/v1' + path)).json();
    game = await post(context, path, { action: 'cancel', version: state.version });
    game = await post(context, path, { action: 'create', kind: 'undercover', language: 'en', version: game.version });
    for (const c of [context, ...guests]) game = await post(c, path, { action: 'join', version: game.version });
    await post(context, path, { action: 'start', version: game.version });
    await page.getByRole('button', { name: 'Show my word', exact: true }).click();
    await expect(page.locator('.secret-word strong')).toBeVisible();
    await page.waitForResponse(r => r.url().endsWith(path) && r.request().method() === 'GET');
    await expect(page.locator('.secret-word strong')).toBeVisible();
    await page.locator('.tabs').getByRole('button', { name: 'Content', exact: true }).click();
    await page.locator('.tabs').getByRole('button', { name: 'Games', exact: true }).click();
    await expect(page.locator('.secret-word strong')).toHaveCount(0);
    await accessible(page);
  } finally { await Promise.all(guests.map(g => g.close())); }
});

