import {expect,type BrowserContext} from '@playwright/test';
let cookies:Awaited<ReturnType<BrowserContext['cookies']>>=[];
// Feature tests share one login; the dedicated login test still covers the real form.
export async function hostSession(context:BrowserContext){
 if(cookies.length){await context.addCookies(cookies);return;}
 const response=await context.request.post('/api/v1/login',{headers:{'X-RoomDeck-Request':'1'},data:{username:'e2e-host',password:'test-only-strong-password-2026'}});
 expect(response.ok(),await response.text()).toBeTruthy();
 cookies=(await context.cookies()).filter(cookie=>cookie.name==='rd_host');
}
