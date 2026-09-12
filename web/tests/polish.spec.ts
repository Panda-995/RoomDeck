import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { hostSession } from "./host-session";

test("custom menus support pointer, keyboard, modal layering and mobile bounds", async ({
  page,
  context,
}) => {
  await hostSession(context);
  const response = await context.request.post("/api/v1/rooms", {
    headers: { "X-RoomDeck-Request": "1" },
    data: { name: "周末，在一起", duration: 7200, retention: 86400 },
  });
  expect(response.ok()).toBeTruthy();
  const room = await response.json();
  await page.goto("/room/" + room.id);
  await page.locator("#language").click();
  const menu = page.locator(".select-menu:popover-open");
  await expect(menu).toBeVisible();
  await menu.getByRole("option", { name: "English", exact: true }).click();
  await expect(page.locator("#language")).toHaveValue("en");
  await page.locator("#language").focus();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Home");
  await page.keyboard.press("Enter");
  await expect(page.locator("#language")).toHaveValue("zh");
  await page.getByRole("button", { name: "设置", exact: true }).last().click();
  const select = page.getByRole("dialog").locator("select");
  await select.click();
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("option")).toHaveCount(3);
  await menu.getByRole("option").nth(1).click();
  await expect(select).toHaveValue("604800");
  await select.click();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(page.getByRole("dialog")).toBeVisible();
  for (const width of [390, 1280]) {
    await page.setViewportSize({ width, height: 844 });
    await select.click();
    const box = await menu.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    expect(box!.y + box!.height).toBeLessThanOrEqual(844);
    await page.evaluate(() =>
      Promise.all(
        document
          .getAnimations()
          .filter((a) => a.effect?.getTiming().iterations !== Infinity)
          .map((a) => a.finished.catch(() => {})),
      ),
    );
    await page.screenshot({ path: `test-results/polish-menu-${width}.png` });
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await page.keyboard.press("Escape");
  }
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.locator("#language").click();
  await page.locator(".room-heading h1").click();
  await expect(menu).toHaveCount(0);
  const pair = await context.request.post(
    `/api/v1/rooms/${room.id}/display-session`,
    { headers: { "X-RoomDeck-Request": "1" }, data: {} },
  );
  await page.goto("/display#pair=" + (await pair.json()).token);
  await expect(page.locator(".display-qr")).toBeVisible();
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollHeight <= innerHeight + 1,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `test-results/polish-welcome-${size.width}.png`,
    });
  }
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
    ).violations,
  ).toEqual([]);
});
