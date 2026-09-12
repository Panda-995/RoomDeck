import { test, expect } from '@playwright/test';

test('offline recovery preserves drafts and resynchronizes room content', async ({ context, page }) => {
  const post = async (path: string, data: unknown) => {
    const response = await context.request.post('/api/v1' + path, { headers: { 'X-RoomDeck-Request': '1' }, data });
    expect(response.ok(), await response.text()).toBeTruthy();
    return response.json();
  };
  await post('/login', { username: 'e2e-host', password: 'test-only-strong-password-2026' });
  const room = await post('/rooms', { name: 'Recovery verification', duration: 7200, retention: 86400 });
  await page.goto('/room/' + room.id);
  await page.locator('#language').selectOption('zh');
  await page.getByRole('button', { name: '分享内容', exact: true }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: '便签', exact: true }).click();
  await page.getByRole('dialog').locator('textarea').fill('断网期间保留的草稿');
  await context.setOffline(true);
  await expect(page.locator('.live-status')).toContainText('网络已断开');
  await expect(page.getByRole('dialog').locator('textarea')).toHaveValue('断网期间保留的草稿');
  await context.setOffline(false);
  await expect(page.locator('.live-status')).not.toContainText('网络已断开');
  await page.reload();
  await page.getByRole('button', { name: '分享内容', exact: true }).first().click();
  await expect(page.getByRole('dialog').locator('textarea')).toHaveValue('断网期间保留的草稿');
  await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
  await post('/rooms/' + room.id + '/contents', { kind: 'note', title: '同步后的内容', body: '恢复成功' });
  await expect(page.locator('.content-card')).toContainText('同步后的内容');
});
