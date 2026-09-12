import { test, expect, type Page } from '@playwright/test';
import {hostSession} from './host-session';

async function room(page: Page) {
  const headers = { 'X-RoomDeck-Request': '1' };
  await hostSession(page.context());
  const response = await page.request.post('/api/v1/rooms', { headers, data: { name: 'Motion review 动效检查', duration: 7200, retention: 86400 } });
  expect(response.ok()).toBe(true);
  const value = await response.json();
  await page.goto('/room/' + value.id);
  await page.locator('#language').selectOption('en');
  await expect(page.locator('.feed')).toBeVisible();
}
async function settle(page: Page) {
  await page.evaluate(() => Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))));
}

test('motion retargets navigation, preserves its position and animates both dialog directions', async ({ page }) => {
  await room(page);
  await settle(page);
  const nav = page.locator('.tabs');
  const position = await nav.boundingBox();
  await nav.getByRole('button', { name: 'Games', exact: true }).click();
  await nav.getByRole('button', { name: 'Screen sharing', exact: true }).click();
  await nav.getByRole('button', { name: 'Content', exact: true }).click();
  await settle(page);
  const finalPosition = await nav.boundingBox();
  expect(Math.abs(position!.y - finalPosition!.y)).toBeLessThan(1);
  const ink = await nav.locator('.nav-highlight').boundingBox();
  const active = await nav.getByRole('button', { name: 'Content', exact: true }).boundingBox();
  for (const key of ['x','y','width','height'] as const) expect(Math.abs(ink![key] - active![key])).toBeLessThan(1);
  await page.getByRole('button', { name: 'Share something', exact: true }).first().click();
  const entry = await page.getByRole('dialog').evaluate(el => el.getAnimations().map(a => ({ timing: a.effect!.getTiming(), frames: (a.effect as KeyframeEffect).getKeyframes() })));
  expect(entry.some(a => a.timing.duration === 260 && a.frames[0].opacity === '0')).toBe(true);
  await settle(page);
  await page.getByRole('button', { name: 'Close', exact: true }).click();
  await expect(page.locator('dialog[data-leaving]')).toHaveCount(1);
  const exit = await page.locator('dialog[data-leaving]').evaluate(el => el.getAnimations().map(a => a.effect!.getTiming().duration));
  expect(exit).toContain(180);
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(await page.locator('html').getAttribute('class')).not.toContain('modal-open');
});

test('reduced motion and keyboard input keep navigation and dialogs immediate', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await room(page);
  await page.locator('.tabs').getByRole('button', { name: 'Games', exact: true }).click();
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.locator('.tabs').getByRole('button', { name: 'Content', exact: true }).click();
  await page.getByRole('button', { name: 'Share something', exact: true }).first().click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await page.locator('.tabs').getByRole('button', { name: 'Games', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('html')).toHaveAttribute('data-input','keyboard');
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.locator('.tabs').getByRole('button', { name: 'Content', exact: true }).focus();
  await page.keyboard.press('Enter');
  await page.getByRole('button', { name: 'Share something', exact: true }).first().focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
