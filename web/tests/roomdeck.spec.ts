import {test,expect} from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
test('bilingual multi-browser sharing, moderation, voting and export',async({browser,page,baseURL})=>{
 const initial=await (await page.request.get('/api/v1/status')).json();const token=initial.setup_required?fs.readFileSync(process.env.ROOMDECK_SETUP_TOKEN_FILE||'../.local/e2e-data/setup-token','utf8').trim():'';
 const post=async(url:string,body:unknown)=>page.request.post('/api/v1'+url,{headers:{'X-RoomDeck-Request':'1'},data:body});
 await page.goto(initial.setup_required?'/setup':'/login');await page.locator('#language').selectOption('en');
 if(initial.setup_required)await page.getByLabel('One-time setup token').fill(token);await page.getByLabel('Username',{exact:true}).fill('e2e-host');await page.getByLabel(/^Password/).fill('test-only-strong-password-2026');await page.getByRole('button',{name:initial.setup_required?'Create administrator':'Sign in'}).click();
 await expect(page.getByRole('heading',{name:'Your rooms'})).toBeVisible();await page.getByRole('button',{name:'Create a room'}).first().click();await page.getByLabel('Room name',{exact:true}).fill('Friday Together');await page.getByRole('button',{name:'Create & invite'}).click();await page.waitForURL('**/room/**');await expect(page.getByRole('dialog')).toBeVisible();await page.getByRole('button',{name:'Close',exact:true}).click();
 const roomID=new URL(page.url()).pathname.split('/').pop()!;
 const snap=await (await page.request.get(`/api/v1/rooms/${roomID}/snapshot`)).json();
 const guestContext=await browser.newContext({baseURL,viewport:{width:390,height:844}});const guest=await guestContext.newPage();
 await guest.goto('/join?code='+snap.room.code);await guest.locator('#language').selectOption('zh');await expect(guest.getByRole('heading',{name:'Friday Together'})).toBeVisible();await guest.getByLabel('怎么称呼你？').fill('小林');await guest.getByRole('button',{name:'进入房间',exact:true}).click();await expect(guest.getByRole('button',{name:'分享内容'}).last()).toBeEnabled();
 await guest.getByRole('button',{name:'分享内容'}).last().click();await guest.getByRole('dialog').getByRole('button',{name:'照片',exact:true}).click();await guest.locator('input[type=file]').setInputFiles(path.resolve('../prototype/assets/friends.jpg'));await expect(guest.getByText('已分享',{exact:true})).toBeVisible({timeout:15000});await guest.getByRole('button',{name:'关闭',exact:true}).click();
 await expect(page.locator('.content-card.photo')).toHaveCount(1);await page.locator('.content-card.photo').getByRole('button',{name:'Add to display',exact:true}).click();
 const paired=await (await post(`/rooms/${roomID}/display-session`,{})).json();
 const displayContext=await browser.newContext({baseURL,viewport:{width:1600,height:1000}});const display=await displayContext.newPage();await display.goto('/display#pair='+paired.token);await display.locator('#language').selectOption('zh');await expect(display.locator('.display-invitation')).toBeVisible();
 await page.locator('.display-modes').getByRole('button',{name:'Notes & links',exact:true}).click();await expect(display.getByText('等待便签或链接加入上屏',{exact:true})).toBeVisible();
 await page.locator('.display-modes').getByRole('button',{name:'Photo slideshow',exact:true}).click();await expect(display.locator('.display-photo')).toBeVisible();
 const frozenSource=await display.locator('.display-photo').getAttribute('src');
 await page.getByRole('button',{name:'Pause slideshow',exact:true}).click();
 const second=Math.floor(Date.now()/1000);await expect.poll(()=>Math.floor(Date.now()/1000)).toBeGreaterThan(second);
 const secondImage=fs.readFileSync(path.resolve('../prototype/assets/table.jpg'));
 const secondUpload=await guest.request.post(`/api/v1/rooms/${roomID}/assets?kind=photo&name=second.jpg&size=${secondImage.length}`,{headers:{'X-RoomDeck-Request':'1','Content-Type':'application/octet-stream'},data:secondImage});expect(secondUpload.status()).toBe(201);
 const secondAsset=await secondUpload.json();
 await page.request.patch(`/api/v1/rooms/${roomID}/contents/${secondAsset.id}`,{headers:{'X-RoomDeck-Request':'1'},data:{selected:true}});
 await expect(display.locator('.display-caption')).toContainText('/ 2');
 await expect(display.locator('.display-photo')).toHaveAttribute('src',frozenSource!);
 for(const size of [{width:1280,height:720},{width:1920,height:1080}]){await display.setViewportSize(size);const canvas=await display.locator('.display-canvas').boundingBox();const caption=await display.locator('.display-caption').boundingBox();expect(caption!.y+caption!.height).toBeLessThanOrEqual(canvas!.y+canvas!.height-16);expect(await display.evaluate(()=>document.documentElement.scrollHeight<=innerHeight+1)).toBe(true);}
 await page.screenshot({path:'test-results/host-active-en.png',fullPage:true});await display.screenshot({path:'test-results/display-active-zh.png',fullPage:true});

 await guest.getByRole('button',{name:'分享内容'}).last().click();await guest.getByRole('dialog').getByRole('button',{name:'便签',exact:true}).click();await guest.getByLabel('想告诉大家什么').fill('合照在门口拍 — See you outside');await guest.getByRole('button',{name:'分享到房间'}).click();await page.getByRole('button',{name:'Notes',exact:true}).click();await expect(page.getByText('合照在门口拍 — See you outside')).toBeVisible();
 await page.getByRole('button',{name:'Share something',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Polls',exact:true}).click();await page.getByLabel('Question',{exact:true}).fill('Where next?');await page.getByLabel('Options (one per line)').fill('Park\nCafe');await page.getByRole('button',{name:'Share with room'}).click();
 await guest.locator('.filter-chips').getByRole('button',{name:'投票',exact:true}).click();await expect(guest.getByRole('heading',{name:'Where next?'})).toBeVisible();await guest.getByRole('button',{name:'○ Park'}).click();await guest.getByRole('button',{name:'提交投票',exact:true}).click();await expect(guest.getByText('1 人已投票')).toBeVisible();
 await page.getByRole('button',{name:'Blank screen',exact:true}).click();await expect(display.locator('.blank-screen')).toBeVisible();await expect(display.locator('.display-photo')).toHaveCount(0);await expect(display.locator('.display-qr')).toHaveCount(0);
 await page.locator('#language').selectOption('zh');await expect(page.getByRole('button',{name:'现场控制',exact:true}).first()).toBeVisible();await page.getByRole('button',{name:'导出房间',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'导出房间',exact:true}).click();await expect(page.getByRole('link',{name:/下载打包文件/})).toBeVisible({timeout:15000});const downloadLink=await page.getByRole('link',{name:/下载打包文件/}).getAttribute('href');const archive=await page.request.get(downloadLink!);expect(archive.status()).toBe(200);expect((await archive.body()).subarray(0,2).toString()).toBe('PK');await page.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('button',{name:'结束活动',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'结束活动',exact:true}).click();await expect(guest.getByRole('heading',{name:'今晚，收获满满。'})).toBeVisible();await expect(display.getByText('谢谢你来，一起留住了此刻。')).toBeVisible();
 await page.screenshot({path:'test-results/host-zh.png',fullPage:true});await guest.screenshot({path:'test-results/guest-zh.png',fullPage:true});
 await guest.locator('#language').selectOption('en');await expect(guest.getByRole('heading',{name:'A moment worth keeping.'})).toBeVisible();await guest.screenshot({path:'test-results/guest-en.png',fullPage:true});
 for(const width of [320,390,768]){await guest.setViewportSize({width,height:900});expect(await guest.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);}
 await guestContext.close();await displayContext.close();
});
