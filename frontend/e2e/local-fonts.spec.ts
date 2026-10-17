import { expect, test } from "@playwright/test";

test("bundled UI fonts load with external requests blocked", async ({ page, baseURL }) => {
  const external: string[] = [];
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin === new URL(baseURL!).origin) {
      await route.continue();
    } else {
      external.push(url.href);
      await route.abort();
    }
  });
  await page.goto("/");
  const fonts = await page.evaluate(async () => {
    const faces = await Promise.all([
      document.fonts.load('400 16px "Inter"'),
      document.fonts.load('700 16px "Inter"'),
      document.fonts.load('400 16px "JetBrains Mono"'),
    ]);
    return faces.map((group) => group.map((face) => face.status));
  });
  expect(fonts).toEqual([["loaded"], ["loaded"], ["loaded"]]);
  await expect(page.locator("body")).toContainText("Sessions");
  expect(external).toEqual([]);
});
