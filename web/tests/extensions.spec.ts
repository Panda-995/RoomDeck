import AxeBuilder from '@axe-core/playwright';
import {test,expect,type BrowserContext} from '@playwright/test';
import {hostSession} from './host-session';
async function post(c:BrowserContext,path:string,data:unknown){const r=await c.request.post('/api/v1'+path,{headers:{'X-RoomDeck-Request':'1'},data});expect(r.ok(),await r.text()).toBeTruthy();return r.json();}
async function fixture(c:BrowserContext){await hostSession(c);return post(c,'/rooms',{name:'新功能验收 · Together',duration:7200,retention:86400});}

test('interactions, approval queue and explicit hidden poll display across roles',async({context,page,browser,baseURL})=>{
 const room=await fixture(context);const guest=await browser.newContext({baseURL,viewport:{width:390,height:844}});const displayContext=await browser.newContext({baseURL});
 try{
 await post(guest,'/join',{code:room.code,name:'小林'});
 const note=await post(guest,`/rooms/${room.id}/contents`,{kind:'note',title:'聚会留言',body:'等大家一起合照'});
 const poll=await post(context,`/rooms/${room.id}/contents`,{kind:'poll',title:'接下来玩什么？',poll:{options:['谁是卧底','骰子猜点数'],multiple:false,max_choices:1,hide_results:true,closed:false,closes_at:Math.floor(Date.now()/1000)+600}});
 await page.goto('/room/'+room.id);await page.locator('#language').selectOption('zh');
 await page.locator('.interaction-panel summary').click();
 await page.getByRole('combobox',{name:'弹幕模式',exact:true}).selectOption('approval');
 const gp=await guest.newPage();await gp.goto('/room/'+room.id);await gp.locator('#language').selectOption('zh');await gp.locator('.interaction-panel summary').click();
 await gp.getByLabel('弹幕内容（最多 60 字）').fill('大家晚上好');await gp.getByRole('button',{name:'发送弹幕',exact:true}).click();
 await expect(page.locator('.danmaku-list')).toContainText('大家晚上好');await page.locator('.danmaku-list').getByRole('button',{name:'批准'}).click();
 const card=gp.locator('.content-card').filter({hasText:'聚会留言'});await card.getByRole('button',{name:'表情',exact:true}).click();await card.getByRole('button',{name:'喜欢',exact:true}).click();await expect(card.getByRole('button',{name:'喜欢: 1'})).toHaveAttribute('aria-pressed','true');
 await page.locator('.display-modes').getByRole('button',{name:'投票',exact:true}).click();await page.getByRole('combobox',{name:'选择上屏投票',exact:true}).selectOption(poll.id);
 const pair=await post(context,`/rooms/${room.id}/display-session`,{});const dp=await displayContext.newPage();await dp.goto('/display#pair='+pair.token);await dp.locator('#language').selectOption('zh');
 await expect(dp.locator('.display-poll')).toContainText('接下来玩什么');await expect(dp.locator('.display-poll-option')).toHaveCount(2);await expect(dp.locator('.display-poll .poll-bar')).toHaveCount(0);
 await expect(dp.locator('.display-danmaku')).toContainText('大家晚上好');
 for(const size of [{width:1920,height:1080},{width:1280,height:720}]){await dp.setViewportSize(size);expect(await dp.evaluate(()=>document.documentElement.scrollHeight<=innerHeight+1)).toBe(true);}
 await dp.screenshot({path:'test-results/features-display.png',fullPage:true});
 await page.locator('.tabs').getByRole('button',{name:'投屏',exact:true}).click();await page.getByRole('combobox',{name:'投屏方式',exact:true}).selectOption('approval');
 await gp.locator('.guest-bottom').getByRole('button',{name:'投屏',exact:true}).click();await gp.getByRole('button',{name:'申请投屏',exact:true}).click();
 await expect(page.locator('.queue-list')).toContainText('小林');await page.locator('.queue-list').getByRole('button',{name:'批准',exact:true}).click();await expect(gp.locator('.screen-queue')).toContainText('轮到你了');
 await gp.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));await gp.screenshot({path:'test-results/features-queue-mobile.png',fullPage:true});
 await page.locator('.tabs').getByRole('button',{name:'内容',exact:true}).click();await page.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));await page.screenshot({path:'test-results/features-host.png',fullPage:true});expect((await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze()).violations).toEqual([]);
 await gp.locator('.guest-bottom').getByRole('button',{name:'内容',exact:true}).click();await gp.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));await gp.screenshot({path:'test-results/features-mobile.png',fullPage:true});expect((await new AxeBuilder({page:gp}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze()).violations).toEqual([]);
 const res=await context.request.patch(`/api/v1/rooms/${room.id}/contents/${poll.id}`,{headers:{'X-RoomDeck-Request':'1'},data:{visible:false}});expect(res.ok()).toBeTruthy();await expect(dp.locator('.display-poll')).toHaveCount(0);
 expect(note.id).toBeTruthy();
 }finally{await guest.close();await displayContext.close();}
});

test('refresh resumes only missing upload chunks',async({context,page})=>{
 const room=await fixture(context);const file={name:'resume.bin',mimeType:'application/octet-stream',buffer:Buffer.alloc(6*1024*1024,37)};const indices:string[]=[];let failed=false;
 await page.route('**/uploads/*/parts/*',async route=>{const index=route.request().url().split('/').pop()!;indices.push(index);if(index==='1'&&!failed){failed=true;await route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({error:{code:'INTERNAL_ERROR'}})});}else await route.continue();});
 await page.goto('/room/'+room.id);await page.locator('#language').selectOption('zh');await page.getByRole('button',{name:'分享内容',exact:true}).first().click();await page.getByRole('dialog').getByRole('button',{name:'文件',exact:true}).click();await page.locator('input[type=file]').setInputFiles(file);
 await expect(page.locator('.upload-list')).toContainText('上传失败');await page.reload();await page.getByRole('button',{name:'分享内容',exact:true}).first().click();await expect(page.locator('.upload-list')).toContainText('重新选择原文件');await page.getByRole('button',{name:'重新选择原文件',exact:true}).click();await page.locator('input[type=file]').setInputFiles(file);await expect(page.locator('.upload-list')).toContainText('已分享');expect(indices).toEqual(['0','1','1']);
 await page.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));await page.screenshot({path:'test-results/features-upload.png',fullPage:false});
});
