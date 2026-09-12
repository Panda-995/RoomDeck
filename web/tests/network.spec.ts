import { test, expect, type Route } from '@playwright/test';
import fs from 'node:fs';
const en=JSON.parse(fs.readFileSync('src/locales/en.json','utf8'));
const zh=JSON.parse(fs.readFileSync('src/locales/zh.json','utf8'));

test('stalled requests time out, restore controls and remain bilingual',async({page})=>{
 let held:Route|undefined;
 await page.route('**/api/v1/status',r=>r.fulfill({json:{setup_required:false,authenticated:false}}));
 await page.route('**/api/v1/login',r=>{held=r;});
 await page.goto('/login');await page.locator('#language').selectOption('en');
 await page.getByLabel('Username',{exact:true}).fill('network-test');
 await page.getByLabel(/^Password/).fill('test-password-12345');
 await page.clock.install();await page.clock.pauseAt(new Date());
 await page.getByRole('button',{name:'Sign in',exact:true}).click();
 await expect.poll(()=>!!held).toBe(true);
 await page.clock.fastForward(15_001);
 await expect(page.getByRole('alert')).toHaveText(en.error.NETWORK_TIMEOUT);
 await expect(page.getByRole('button',{name:'Sign in',exact:true})).toBeEnabled();
 await page.locator('#language').selectOption('zh');
 await expect(page.getByRole('alert')).toHaveText(zh.error.NETWORK_TIMEOUT);
 await held?.abort().catch(()=>{});
});

test('HTML proxy errors are surfaced without leaving controls busy',async({page})=>{
 await page.route('**/api/v1/status',r=>r.fulfill({json:{setup_required:false,authenticated:false}}));
 await page.route('**/api/v1/login',r=>r.fulfill({status:502,contentType:'text/html',body:'<h1>Bad Gateway</h1>'}));
 await page.goto('/login');await page.locator('#language').selectOption('en');
 await page.getByLabel('Username',{exact:true}).fill('network-test');await page.getByLabel(/^Password/).fill('test-password-12345');
 await page.getByRole('button',{name:'Sign in',exact:true}).click();
 await expect(page.getByRole('alert')).toHaveText(en.error.NETWORK_ERROR);
 await expect(page.getByRole('button',{name:'Sign in',exact:true})).toBeEnabled();
});
