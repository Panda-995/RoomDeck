import {test,expect,chromium,type BrowserContext} from '@playwright/test';
import fs from 'node:fs';
test('games: private words, dice guesses and mobile layout',async({browser,baseURL})=>{
 const contexts:BrowserContext[]=[];
 try{
  const host=await browser.newContext({baseURL});contexts.push(host);
  const post=async(c:BrowserContext,path:string,data:unknown)=>{const r=await c.request.post('/api/v1'+path,{headers:{'X-RoomDeck-Request':'1'},data});expect(r.ok(),await r.text()).toBeTruthy();return r.json();};
  const status=await(await host.request.get('/api/v1/status')).json();
  if(status.setup_required)await post(host,'/setup',{token:fs.readFileSync(process.env.ROOMDECK_SETUP_TOKEN_FILE||'../.local/e2e-data/setup-token','utf8').trim(),username:'e2e-host',password:'test-only-strong-password-2026'});
  else await post(host,'/login',{username:'e2e-host',password:'test-only-strong-password-2026'});
  const room=await post(host,'/rooms',{name:'Games night 游戏之夜',duration:7200,retention:86400});
  for(let i=0;i<3;i++){const c=await browser.newContext({baseURL,viewport:{width:390,height:844}});contexts.push(c);await post(c,'/join',{code:room.code,name:['小林','Alex','Mia'][i]});}
  const page=await host.newPage();await page.goto('/room/'+room.id);await page.locator('#language').selectOption('en');await page.locator('.tabs').getByRole('button',{name:'Games',exact:true}).click();
  await page.locator('.game-library article').filter({has:page.getByRole('heading',{name:'Guess the dice',exact:true})}).getByRole('button',{name:'Create game'}).click();
  await page.getByRole('button',{name:'Take a seat'}).click();
  const guest=await contexts[1].newPage();await guest.goto('/room/'+room.id);await guest.locator('#language').selectOption('zh');await guest.locator('.guest-bottom').getByRole('button',{name:'游戏',exact:true}).click();await guest.getByRole('button',{name:'我要加入',exact:true}).click();
  await expect(page.getByRole('button',{name:'Start game'})).toBeEnabled();await page.getByRole('button',{name:'Start game'}).click();await page.getByRole('button',{name:'Guess 3',exact:true}).click();await expect(page.getByRole('button',{name:'Confirm choice'})).toBeEnabled();await page.getByRole('button',{name:'Confirm choice'}).click();await guest.getByRole('button',{name:'猜 4 点',exact:true}).click();await guest.getByRole('button',{name:'确认选择'}).click();
  await expect(guest.getByText(/掷出了 [1-6] 点/)).toBeVisible();await page.screenshot({path:'test-results/dice-en.png',fullPage:true});await guest.screenshot({path:'test-results/dice-zh-mobile.png',fullPage:true});
  const gameURL='/rooms/'+room.id+'/game';let g=await(await host.request.get('/api/v1'+gameURL)).json();g=await post(host,gameURL,{action:'cancel',version:g.version});g=await post(host,gameURL,{action:'create',kind:'undercover',language:'zh',version:g.version});
  for(const c of contexts)g=await post(c,gameURL,{action:'join',version:g.version});g=await post(host,gameURL,{action:'start',version:g.version});
  for(const c of contexts){const v=await(await c.request.get('/api/v1'+gameURL)).json();expect(v.mine.word).toBeTruthy();for(const p of v.players){expect(p.word).toBeUndefined();expect(p.spy).toBeUndefined();}}
  await expect(guest.getByRole('button',{name:'查看我的词'})).toBeVisible();await guest.getByRole('button',{name:'查看我的词'}).click();await expect(guest.locator('.secret-word strong')).toBeVisible();await guest.reload();await guest.locator('.guest-bottom').getByRole('button',{name:'游戏',exact:true}).click();await expect(guest.getByRole('button',{name:'查看我的词'})).toBeVisible();await expect(guest.locator('.secret-word strong')).toHaveCount(0);
  await guest.screenshot({path:'test-results/undercover-zh.png',fullPage:true});
  for(const width of [320,390,768]){await guest.setViewportSize({width,height:900});expect(await guest.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);}
 }finally{await Promise.all(contexts.map(c=>c.close()));}
});

test('real SFU: desktop capture to two viewers and display, then stop',async({baseURL})=>{
 test.skip(process.env.ROOMDECK_MEDIA_TEST!=='1','Requires a real LiveKit process');test.setTimeout(90000);
 const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,args:['--enable-usermedia-screen-capturing','--auto-select-desktop-capture-source=Entire screen','--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream']});
 try{
 const host=await browser.newContext({baseURL});const post=async(c:BrowserContext,path:string,data:unknown)=>{const r=await c.request.post('/api/v1'+path,{headers:{'X-RoomDeck-Request':'1'},data});expect(r.ok(),await r.text()).toBeTruthy();return r.json();};
 await post(host,'/login',{username:'e2e-host',password:'test-only-strong-password-2026'});const room=await post(host,'/rooms',{name:'Screen together 一起看',duration:7200,retention:86400});
 const publisher=await host.newPage();publisher.on('console',msg=>{if(msg.type()==='error')console.log('publisher:',msg.text());});await publisher.goto('/room/'+room.id);await publisher.locator('#language').selectOption('en');await publisher.locator('.tabs').getByRole('button',{name:'Screen sharing',exact:true}).click();await publisher.getByRole('button',{name:'Share my screen'}).click();
 await expect(publisher.locator('.screen-media video')).toHaveCount(1,{timeout:25000});
 const viewers=[];
 for(const name of ['Viewer A','Viewer B']){const c=await browser.newContext({baseURL});await post(c,'/join',{code:room.code,name});const page=await c.newPage();await page.goto('/room/'+room.id);await page.locator('#language').selectOption('en');await page.locator('.tabs').getByRole('button',{name:'Screen sharing',exact:true}).click();await page.getByRole('button',{name:'Watch screen'}).click();await expect(page.locator('.screen-media video')).toHaveCount(1,{timeout:20000});await expect.poll(()=>page.locator('.screen-media video').evaluate((v:HTMLVideoElement)=>v.videoWidth)).toBeGreaterThan(0);viewers.push(page);}
 const pair=await post(host,'/rooms/'+room.id+'/display-session',{});const displayContext=await browser.newContext({baseURL});const display=await displayContext.newPage();await display.goto('/display#pair='+pair.token);await expect(display.locator('.screen-media video')).toHaveCount(1,{timeout:20000});await expect.poll(()=>display.locator('.screen-media video').evaluate((v:HTMLVideoElement)=>v.videoWidth)).toBeGreaterThan(0);
 await viewers[0].screenshot({path:'test-results/screen-viewer.png',fullPage:true});await display.screenshot({path:'test-results/screen-display.png',fullPage:true});
 const selectedPoll=await post(host,'/rooms/'+room.id+'/contents',{kind:'poll',title:'Live poll while sharing',poll:{options:['A','B'],multiple:false,max_choices:1,hide_results:true,closed:false,closes_at:Math.floor(Date.now()/1000)+600}});
 let snapshot=await(await host.request.get('/api/v1/rooms/'+room.id+'/snapshot')).json();
 await post(host,'/rooms/'+room.id+'/display/control',{mode:'poll',target:selectedPoll.id,source:'content',expected_version:snapshot.room.version});
 await expect(display.locator('.screen-media video')).toHaveCount(0,{timeout:15000});await expect(display.locator('.display-poll')).toContainText('Live poll while sharing');
 for(const viewer of viewers)await expect(viewer.locator('.screen-media video')).toHaveCount(1);
 snapshot=await(await host.request.get('/api/v1/rooms/'+room.id+'/snapshot')).json();
 await post(host,'/rooms/'+room.id+'/display/control',{mode:'poll',target:selectedPoll.id,source:'screen',expected_version:snapshot.room.version});
 await expect(display.locator('.screen-media video')).toHaveCount(1,{timeout:20000});
  await publisher.getByRole('button',{name:'Stop sharing for everyone'}).click();for(const page of viewers)await expect(page.locator('.screen-media video')).toHaveCount(0,{timeout:10000});await expect(display.locator('.screen-media video')).toHaveCount(0,{timeout:10000});
 // A guest can take the next turn. The host can stop it without being the publisher.
 await post(host,'/rooms/'+room.id+'/screen/queue',{action:'mode',mode:'approval'});
 await post(viewers[0].context(),'/rooms/'+room.id+'/screen/queue',{action:'request'});
 const queue=await(await host.request.get('/api/v1/rooms/'+room.id+'/screen/queue')).json();
 await post(host,'/rooms/'+room.id+'/screen/queue',{action:'approve',id:queue.requests[0].id});
 await expect(viewers[0].locator('.screen-queue')).toContainText('Your turn');
 await viewers[0].getByRole('button',{name:'Share my screen'}).click();await expect(viewers[0].locator('.screen-media video')).toHaveCount(1,{timeout:20000});await expect(display.locator('.screen-media video')).toHaveCount(1,{timeout:20000});
 await publisher.getByRole('button',{name:'Stop sharing for everyone'}).click();await expect(viewers[0].locator('.screen-media video')).toHaveCount(0,{timeout:10000});
 }finally{await browser.close();}
});
