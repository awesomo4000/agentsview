// @vitest-environment jsdom
import { beforeEach, afterEach, expect, it, vi } from "vite-plus/test";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/svelte";
import SessionAnnotations from "./SessionAnnotations.svelte";
import { SessionsService } from "../../api/generated/index.js";
import { settings } from "../../stores/settings.svelte.js";
vi.mock("../../api/generated/index.js", () => ({
  SessionsService: {
    getApiV1SessionsByIdSummary: vi.fn(),
    putApiV1SessionsByIdSummary: vi.fn(),
    patchApiV1SessionsByIdLabels: vi.fn(),
  },
}));
vi.mock("../../stores/sessions.svelte.js", () => ({
  sessions: { refreshActiveSession: vi.fn(), refreshSidebarIfAttached: vi.fn() },
}));
const api = vi.mocked(SessionsService);
beforeEach(() => {
  vi.resetAllMocks();
  settings.readOnly = false;
  api.getApiV1SessionsByIdSummary.mockResolvedValue({
    session_id: "one",
    content: "Existing summary",
    updated_at: "",
  });
});
afterEach(cleanup);
it("adds and removes tags through incremental updates", async () => {
  api.patchApiV1SessionsByIdLabels
    .mockResolvedValueOnce({ session_id: "one", session_found: true, labels: ["auth", "kept"] })
    .mockResolvedValueOnce({ session_id: "one", session_found: true, labels: ["kept"] });
  render(SessionAnnotations, { sessionId: "one", labels: ["kept"] });
  await fireEvent.input(screen.getByRole("textbox", { name: "New tag" }), {
    target: { value: " auth " },
  });
  await fireEvent.click(screen.getByRole("button", { name: "Add tag" }));
  await waitFor(() =>
    expect(api.patchApiV1SessionsByIdLabels).toHaveBeenCalledWith({ id: "one" }, { add: ["auth"] }),
  );
  await fireEvent.click(await screen.findByRole("button", { name: "Remove auth" }));
  await waitFor(() =>
    expect(api.patchApiV1SessionsByIdLabels).toHaveBeenLastCalledWith(
      { id: "one" },
      { remove: ["auth"] },
    ),
  );
  await waitFor(() => expect(screen.queryByRole("button", { name: "Remove auth" })).toBeNull());
  expect(screen.getByText("kept")).toBeTruthy();
});
it("retains a failed summary draft and saves it on retry", async () => {
  api.putApiV1SessionsByIdSummary
    .mockRejectedValueOnce(new Error("failed"))
    .mockResolvedValueOnce({ session_id: "one", content: "Updated overview", updated_at: "now" });
  render(SessionAnnotations, { sessionId: "one", labels: [] });
  const field = await screen.findByDisplayValue("Existing summary");
  await fireEvent.input(field, { target: { value: "Updated overview" } });
  await fireEvent.click(screen.getByRole("button", { name: "Save summary" }));
  await screen.findByRole("alert");
  expect((field as HTMLTextAreaElement).value).toBe("Updated overview");
  await fireEvent.click(screen.getByRole("button", { name: "Save summary" }));
  await screen.findByRole("status");
  expect(api.putApiV1SessionsByIdSummary).toHaveBeenLastCalledWith(
    { id: "one" },
    { content: "Updated overview" },
  );
});
it("does not allow overwriting a summary that failed to load", async () => {
  api.getApiV1SessionsByIdSummary.mockRejectedValueOnce(new Error("failed"));
  render(SessionAnnotations, { sessionId: "one", labels: [] });
  await screen.findByRole("alert");
  expect(screen.queryByRole("button", { name: "Save summary" })).toBeNull();
  expect(api.putApiV1SessionsByIdSummary).not.toHaveBeenCalled();
});
it("shows annotations without mutation controls in read-only mode", async () => {
  settings.readOnly = true;
  render(SessionAnnotations, { sessionId: "one", labels: ["kept"] });
  const field = await screen.findByDisplayValue("Existing summary");
  expect((field as HTMLTextAreaElement).disabled).toBe(true);
  expect(screen.queryByRole("button", { name: "Add tag" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Remove kept" })).toBeNull();
});
