import { expect, test } from "@playwright/test";

test("tags and summaries persist and a summary finds its session", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("agentsview-session-vitals", "true"));
  const path = "/sessions/test-session-duration-showcase";
  await page.goto(path);
  const panel = page.getByRole("region", { name: "Session annotations" });
  await expect(panel).toBeVisible();
  await panel.getByRole("textbox", { name: "New tag" }).fill("workstream=local-review");
  await panel.getByRole("button", { name: "Add tag" }).click();
  await expect(panel.getByRole("button", { name: "Remove workstream=local-review" })).toBeVisible();
  const summary = panel.getByRole("textbox", { name: "Session summary" });
  await expect(summary).toBeEnabled();
  await summary.fill("Zephyrquartz summary for the local review workstream.");
  await panel.getByRole("button", { name: "Save summary" }).click();
  await expect(panel.getByRole("status")).toHaveText("Summary saved.");
  await page.reload();
  await expect(summary).toHaveValue("Zephyrquartz summary for the local review workstream.");
  await expect(panel.getByRole("button", { name: "Remove workstream=local-review" })).toBeVisible();
  await page.screenshot({ path: "/tmp/av-session-annotations.png" });
  await page.keyboard.press("ControlOrMeta+k");
  await page.locator(".palette-input").fill("Zephyrquartz");
  const result = page.locator(".palette-item", { hasText: "Zephyrquartz" }).first();
  await expect(result).toBeVisible();
  await result.click();
  await expect(page).toHaveURL(new RegExp(path));
  await panel.getByRole("button", { name: "Remove workstream=local-review" }).click();
  await expect(panel.getByRole("button", { name: "Remove workstream=local-review" })).toHaveCount(
    0,
  );
  await summary.fill("");
  await panel.getByRole("button", { name: "Save summary" }).click();
  await expect(panel.getByRole("status")).toHaveText("Summary saved.");
  await page.reload();
  await expect(summary).toHaveValue("");
  await expect(panel.getByRole("button", { name: "Remove workstream=local-review" })).toHaveCount(
    0,
  );
});
