import { expect, test } from "@playwright/test";

test("a saved semantic preference still finds a session with local FTS", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("agentsview-search-mode", "semantic"));
  await page.goto("/");
  await expect(page.locator(".session-item").first()).toBeVisible();
  await page.keyboard.press("ControlOrMeta+k");
  const input = page.locator(".palette-input");
  await expect(input).toBeVisible();
  const searched = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === "/api/v1/search" && url.searchParams.get("q") === "synchronous DB read";
  });
  await input.fill("synchronous DB read");
  expect((await searched).ok()).toBe(true);
  const result = page.locator(".palette-item", { hasText: "synchronous DB read" }).first();
  await expect(result).toBeVisible();
  await result.click();
  await expect(page).toHaveURL(/\/sessions\/test-session-duration-subagent-1/);
  await expect(page.locator(".breadcrumb-current")).toHaveText(/project-duration/);
});
