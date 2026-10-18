import { expect, test } from "@playwright/test";

test("saved remote settings and clicks cannot send synthetic archive data", async ({ page, baseURL }) => {
  const attempted: string[] = [];
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", async (route) => {
    if (new URL(route.request().url()).origin === origin) await route.continue();
    else { attempted.push(route.request().url()); await route.abort(); }
  });
  await page.addInitScript(() => localStorage.setItem("agentsview-server-url", "http://127.0.0.1:1"));
  const response = await page.goto("/");
  expect(response!.headers()["content-security-policy"]).toContain("form-action 'none'");
  await expect(page.locator("body")).toContainText("Sessions");
  const rejected = await page.evaluate(async () => {
    try { await fetch("http://127.0.0.1:1/synthetic", {method:"POST", body:"synthetic transcript"}); return false; }
    catch { return true; }
  });
  expect(rejected).toBe(true);
  const before = page.url();
  await page.evaluate(() => {
    const link = document.createElement("a");
    link.href = "http://127.0.0.1:1/synthetic-link";
    link.textContent = "synthetic external link";
    link.id = "privacy-probe-link";
    document.body.append(link);
  });
  await page.locator("#privacy-probe-link").click();
  expect(page.url()).toBe(before);
  expect(attempted).toEqual([]);
});
