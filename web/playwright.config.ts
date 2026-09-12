import { defineConfig } from '@playwright/test';
export default defineConfig({
 testDir:'./tests',timeout:60000,fullyParallel:false,workers:1,
 use:{baseURL:process.env.ROOMDECK_TEST_URL||'http://127.0.0.1:8093',headless:true,launchOptions:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE}:undefined},
 reporter:[['list'],['html',{open:'never'}]],
});
