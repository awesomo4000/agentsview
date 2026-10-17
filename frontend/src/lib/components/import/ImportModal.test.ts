// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vite-plus/test";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import ImportModal from "./ImportModal.svelte";
import { m } from "../../i18n/index.js";
import { setLocale } from "../../paraglide/runtime.js";
const { importClaudeAI, importChatGPT } = vi.hoisted(() => ({
  importClaudeAI: vi.fn(),
  importChatGPT: vi.fn(),
}));
vi.mock("../../api/client.js", () => ({ importClaudeAI, importChatGPT }));
beforeEach(() => {
  void setLocale("en", { reload: false });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it.each(["Claude.ai", "ChatGPT"])(
  "imports a local %s archive and reports completion",
  async (provider) => {
    const onimported = vi.fn();
    const importer = provider === "ChatGPT" ? importChatGPT : importClaudeAI;
    importer.mockResolvedValue({ imported: 2, updated: 0, skipped: 0, errors: 0 });
    const { container } = render(ImportModal, { open: true, onclose: vi.fn(), onimported });
    await fireEvent.click(screen.getByRole("button", { name: provider }));
    const file = new File(["synthetic archive"], "conversations.zip", { type: "application/zip" });
    await fireEvent.change(container.querySelector('input[type="file"]')!, {
      target: { files: [file] },
    });
    await fireEvent.click(screen.getByRole("button", { name: m.import_import() }));
    await waitFor(() => expect(onimported).toHaveBeenCalledOnce());
    expect(importer).toHaveBeenCalledWith(
      file,
      expect.objectContaining({
        onProgress: expect.any(Function),
        onIndexing: expect.any(Function),
      }),
    );
    expect(provider === "ChatGPT" ? importClaudeAI : importChatGPT).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: m.import_done() })).toBeTruthy();
  },
);

it("shows a failed file import without reporting success", async () => {
  importClaudeAI.mockRejectedValue(new Error("Invalid archive contents"));
  const onimported = vi.fn();
  const { container } = render(ImportModal, { open: true, onclose: vi.fn(), onimported });
  const file = new File(["{}"], "conversations.json");
  await fireEvent.change(container.querySelector('input[type="file"]')!, {
    target: { files: [file] },
  });
  await fireEvent.click(screen.getByRole("button", { name: m.import_import() }));
  await waitFor(() => expect(screen.getByText("Invalid archive contents")).toBeTruthy());
  expect(onimported).not.toHaveBeenCalled();
});
