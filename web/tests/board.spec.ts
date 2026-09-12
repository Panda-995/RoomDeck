import { test, expect, type BrowserContext, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { hostSession } from "./host-session";
async function post(c: BrowserContext, path: string, data: unknown) {
  const r = await c.request.post("/api/v1" + path, {
    headers: { "X-RoomDeck-Request": "1" },
    data,
  });
  expect(r.ok(), await r.text()).toBeTruthy();
  return r.json();
}
async function openBoard(p: Page, guest = false) {
  await p.getByRole("button", { name: "游戏", exact: true }).last().click();
  await p.getByRole("button", { name: "画板 / 你画我猜", exact: true }).click();
  await expect(p.locator(".board-canvas")).toBeVisible();
}
async function draw(p: Page, x: number, y: number) {
  await p.locator(".board-canvas").scrollIntoViewIfNeeded();
  await expect(p.locator(".board-canvas")).toHaveClass(/can-draw/);
  const box = (await p.locator(".board-canvas").boundingBox())!;
  await p.mouse.move(box.x + box.width * x, box.y + box.height * y);
  await p.mouse.down();
  await p.mouse.move(
    box.x + box.width * (x + 0.1),
    box.y + box.height * (y + 0.1),
    { steps: 12 },
  );
  await p.mouse.up();
  await expect(p.locator(".board-caption")).toContainText("已同步");
}
test("shared board, co-host scopes, private guessing rounds and display", async ({
  context,
  page,
  browser,
  baseURL,
}) => {
  test.setTimeout(90000);
  await hostSession(context);
  const room = await post(context, "/rooms", {
    name: "画在一起 · Draw together",
    duration: 7200,
    retention: 86400,
  });
  const gc = await browser.newContext({
      baseURL,
      viewport: { width: 390, height: 844 },
    }),
    dc = await browser.newContext({
      baseURL,
      viewport: { width: 1280, height: 720 },
    });
  try {
    await post(gc, "/join", { code: room.code, name: "小林" });
    const gp = await gc.newPage();
    await gp.goto("/room/" + room.id);
    await gp.locator("#language").selectOption("zh");
    await page.goto("/room/" + room.id);
    await page.locator("#language").selectOption("zh");
    await page
      .getByRole("button", { name: /^参与者/ })
      .first()
      .click();
    const member = page.locator(".member-row").filter({ hasText: "小林" });
    await member.locator("summary").click();
    for (const scope of ["游戏与画板主持", "现场大屏控制", "内容与弹幕管理"]) {
      await member.getByLabel(scope, { exact: true }).check();
      await expect(member.getByLabel(scope, { exact: true })).toBeChecked();
    }
    await page.screenshot({ path: "test-results/board-permissions.png" });
    await expect(gp.locator(".cohost-banner")).toContainText("游戏与画板主持");
    await page.getByRole("button", { name: "关闭", exact: true }).click();
    await openBoard(page);
    await openBoard(gp, true);
    await draw(gp, 0.15, 0.2);
    await expect(page.locator(".board-canvas polyline")).toHaveCount(1);
    await draw(page, 0.4, 0.2);
    await expect(gp.locator(".board-canvas polyline")).toHaveCount(2);
    await gp
      .getByRole("button", { name: "撤销我的上一笔", exact: true })
      .click();
    await expect(page.locator(".board-canvas polyline")).toHaveCount(1);
    await gc.setOffline(true);
    await expect(gp.locator(".board-panel [role=alert]")).toBeVisible();
    await gc.setOffline(false);
    await expect(gp.locator(".board-panel [role=alert]")).toHaveCount(0);
    await gp.reload();
    await openBoard(gp, true);
    await expect(gp.locator(".board-canvas polyline")).toHaveCount(1);
    await gp.getByRole("button", { name: "创建你画我猜", exact: true }).click();
    await gp.getByRole("button", { name: "确认继续", exact: true }).click();
    await expect(page.locator(".board-round")).toContainText("等待加入");
    await page.getByRole("button", { name: "加入本局", exact: true }).click();
    await gp.getByRole("button", { name: "加入本局", exact: true }).click();
    await gp.getByRole("button", { name: "开始绘画", exact: true }).click();
    await expect(page.locator(".board-round")).toContainText("正在作画");
    await page
      .getByRole("button", { name: "查看我的词语", exact: true })
      .click();
    const word = (await page.locator(".board-secret").textContent())!;
    const gs = await (
      await gc.request.get(`/api/v1/rooms/${room.id}/board`)
    ).json();
    expect(gs.word).toBe("");
    await draw(page, 0.2, 0.2);
    await gp
      .locator(".display-modes")
      .getByRole("button", { name: "画板", exact: true })
      .click();
    const pair = await post(context, `/rooms/${room.id}/display-session`, {});
    const dp = await dc.newPage();
    await dp.goto("/display#pair=" + pair.token);
    await expect(dp.locator(".board-display")).toBeVisible();
    await expect(dp.locator(".board-canvas polyline")).toHaveCount(1);
    expect(
      (
        await (
          await dc.request.get(`/api/v1/rooms/${room.id}/board?display=1`)
        ).json()
      ).word,
    ).toBe("");
    await expect(gp.locator(".board-canvas polyline")).toHaveCount(1);
    for (const size of [
      { width: 1280, height: 720 },
      { width: 1920, height: 1080 },
    ]) {
      await dp.setViewportSize(size);
      const stage = (await dp.locator(".display-canvas").boundingBox())!;
      const scores = (await dp.locator(".board-scores").boundingBox())!;
      expect(scores.y + scores.height).toBeLessThanOrEqual(
        stage.y + stage.height - 16,
      );
      expect(
        await dp.evaluate(
          () => document.documentElement.scrollHeight <= innerHeight + 1,
        ),
      ).toBe(true);
    }
    await gp.screenshot({
      path: "test-results/board-mobile.png",
      fullPage: true,
    });
    await page.screenshot({
      path: "test-results/board-host.png",
      fullPage: true,
    });
    await dp.screenshot({ path: "test-results/board-display.png" });
    await gp.getByLabel("你猜是什么？", { exact: true }).fill(word);
    await gp.getByRole("button", { name: "提交答案", exact: true }).click();
    await expect(dp.locator(".board-round")).toContainText(word);
    await expect(gp.locator(".board-scores")).toContainText("100");
    await gp.getByRole("button", { name: "下一位作画", exact: true }).click();
    await expect(
      page.getByLabel("你猜是什么？", { exact: true }),
    ).toBeVisible();
    await expect(page.locator(".board-secret")).toHaveCount(0);
    await page
      .getByRole("button", { name: /^参与者/ })
      .first()
      .click();
    await member.locator("summary").click();
    await member.getByLabel("游戏与画板主持", { exact: true }).uncheck();
    await expect(
      gp.getByRole("button", { name: "结束本局", exact: true }),
    ).toHaveCount(0);
    await page.getByRole("button", { name: "关闭", exact: true }).click();
    const b = await (
      await gc.request.get(`/api/v1/rooms/${room.id}/board`)
    ).json();
    const denied = await gc.request.post(`/api/v1/rooms/${room.id}/board`, {
      headers: { "X-RoomDeck-Request": "1" },
      data: { action: "finish", epoch: b.epoch, version: b.version },
    });
    expect(denied.status()).toBe(403);
    await gp.locator("#language").selectOption("en");
    expect(
      await gp.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(
      (
        await new AxeBuilder({ page: gp })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
  } finally {
    await gc.close();
    await dc.close();
  }
});
