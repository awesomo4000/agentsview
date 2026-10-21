// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";
import { mount, unmount, tick } from "svelte";
import { ApiError } from "../../api/runtime.js";
import { registerShortcuts } from "../../utils/keyboard.js";

const {
  mockUi,
  mockSessions,
  mockSearchStore,
  mockRouter,
  mockCopyToClipboard,
  mockEmbeddingsService,
} = vi.hoisted(() => ({
  mockUi: {
    activeModal: "commandPalette" as "commandPalette" | null,
    scrollToOrdinal: vi.fn(),
    clearSelection: vi.fn(),
    clearScrollState: vi.fn(),
  },
  mockSessions: {
    activeSessionId: null as string | null,
    sessions: [] as Array<{
      id: string;
      project: string;
      machine: string;
      agent: string;
      first_message: string | null;
      started_at: string | null;
      ended_at: string | null;
      message_count: number;
      user_message_count: number;
      created_at: string;
    }>,
    filters: { project: "" },
    projects: [{ name: "proj-a", session_count: 2 }],
    deselectSession: vi.fn(),
  },
  mockSearchStore: {
    results: [] as Array<unknown>,
    isSearching: false,
    error: null as {
      detail: string | null;
      kind: "generic" | "timeout" | "semantic-unavailable";
    } | null,
    mode: "fulltext" as "fulltext" | "semantic" | "hybrid",
    sort: "relevance" as "relevance" | "recency",
    search: vi.fn(),
    clear: vi.fn(),
    resetSort: vi.fn(),
    retry: vi.fn(),
    setMode: vi.fn(),
    setSort: vi.fn(),
    range: { mode: "relative" as const, days: 0 },
    setRange: vi.fn(),
    resetRange: vi.fn(),
  },
  mockRouter: {
    navigateToSession: vi.fn(),
  },
  mockCopyToClipboard: vi.fn(),
  mockEmbeddingsService: {
    getApiV1EmbeddingsStatus: vi.fn(),
    postApiV1EmbeddingsBuild: vi.fn(),
  },
}));

vi.mock("../../stores/ui.svelte.js", () => ({
  ui: mockUi,
}));

vi.mock("../../stores/sessions.svelte.js", () => ({
  sessions: mockSessions,
}));

vi.mock("../../stores/search.svelte.js", () => ({
  searchStore: mockSearchStore,
}));

vi.mock("../../stores/router.svelte.js", () => ({
  router: mockRouter,
}));

vi.mock("../../stores/messages.svelte.js", () => ({
  messages: {},
}));

vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: mockCopyToClipboard,
}));

// SemanticSetupHelp (mounted for the semantic-unavailable error kind) probes
// the embeddings status on mount; keep the probe pending so palette tests
// exercise the wiring without embeddings API behavior (covered in
// SemanticSetupHelp.test.ts).
vi.mock("../../api/generated/index.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api/generated/index.js")>();
  return {
    ...actual,
    EmbeddingsService: mockEmbeddingsService,
  };
});

// @ts-ignore
import CommandPalette from "./CommandPalette.svelte";

/**
 * Polls via tick() until the selector matches or the iteration limit is hit.
 * Svelte 5's microtask scheduler requires explicit tick() calls to flush DOM
 * updates in jsdom — setTimeout-based waitFor() retries don't drive it.
 */
async function tickUntil(selector: string, maxTicks = 20): Promise<HTMLElement> {
  for (let i = 0; i < maxTicks; i++) {
    await tick();
    const el = document.querySelector<HTMLElement>(selector);
    if (el) return el;
  }
  throw new Error(`"${selector}" not found after ${maxTicks} tick() calls`);
}

function makeSession(id: string, agent: string) {
  return {
    compaction_count: 0,
    consecutive_failure_max: 0,
    edit_churn_count: 0,
    ended_with_role: "",
    final_failure_streak: 0,
    has_peak_context_tokens: false,
    has_total_output_tokens: false,
    mid_task_compaction_count: 0,
    outcome: "",
    outcome_confidence: "",
    secret_leak_count: 0,
    tool_failure_signal_count: 0,
    tool_retry_count: 0,
    id,
    project: "proj-a",
    machine: "mac",
    agent,
    first_message: "hello",
    started_at: "2026-02-20T12:30:00Z",
    ended_at: "2026-02-20T12:31:00Z",
    message_count: 2,
    user_message_count: 1,
    total_output_tokens: 0,
    peak_context_tokens: 0,
    created_at: "2026-02-20T12:30:00Z",
  };
}

function makeSearchResult(overrides: Record<string, unknown> = {}) {
  return {
    session_id: "codex:search123",
    project: "proj-a",
    agent: "codex",
    ordinal: 7,
    timestamp: "2026-01-01T00:00:00Z",
    snippet: "matching content",
    rank: 0,
    snippetFormat: "highlighted-html",
    ...overrides,
  };
}

async function enterSearchQuery(value = "match") {
  const input = document.querySelector<HTMLInputElement>(".palette-input")!;
  input.value = value;
  input.dispatchEvent(new InputEvent("input", { bubbles: true }));
  await tick();
}

describe("CommandPalette", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.body.replaceChildren();
    // jsdom does not implement scrollIntoView
    Element.prototype.scrollIntoView = vi.fn();
    mockSearchStore.results = [];
    mockSearchStore.isSearching = false;
    mockSearchStore.error = null;
    mockSearchStore.mode = "fulltext";
    mockSearchStore.sort = "relevance";
    mockEmbeddingsService.getApiV1EmbeddingsStatus.mockReset();
    mockEmbeddingsService.getApiV1EmbeddingsStatus.mockImplementation(() => new Promise(() => {}));
    mockEmbeddingsService.postApiV1EmbeddingsBuild.mockReset();
    mockSessions.activeSessionId = null;
    mockSessions.deselectSession.mockImplementation(() => {
      mockSessions.activeSessionId = null;
    });
    mockUi.activeModal = "commandPalette";
    mockSessions.filters.project = "";
    mockSessions.sessions = [makeSession("s1", "cursor"), makeSession("s2", "unknown")];
  });

  it("uses agentColor for recent-session dots including fallback", async () => {
    const component = mount(CommandPalette, {
      target: document.body,
    });

    await tick();

    const dots = Array.from(document.querySelectorAll<HTMLElement>(".item-dot"));
    expect(dots).toHaveLength(2);
    expect(dots[0]?.getAttribute("style")).toContain("var(--accent-black)");
    expect(dots[1]?.getAttribute("style")).toContain("var(--accent-blue)");

    unmount(component);
  });

  it("dismisses on an overlay press released on the overlay", async () => {
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    const overlay = document.querySelector<HTMLElement>(".palette-overlay")!;
    overlay.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    overlay.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));

    expect(mockUi.activeModal).toBeNull();

    unmount(component);
  });

  it("stays open when a drag starts in the palette and ends on the overlay", async () => {
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    const input = document.querySelector<HTMLInputElement>(".palette-input")!;
    const overlay = document.querySelector<HTMLElement>(".palette-overlay")!;
    input.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    overlay.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));

    expect(mockUi.activeModal).toBe("commandPalette");

    unmount(component);
  });

  it("stays open when a drag starts on the overlay and ends in the palette", async () => {
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    const input = document.querySelector<HTMLInputElement>(".palette-input")!;
    const overlay = document.querySelector<HTMLElement>(".palette-overlay")!;
    overlay.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    input.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));

    expect(mockUi.activeModal).toBe("commandPalette");

    unmount(component);
  });

  it("calls clear() and resetSort() on unmount via onDestroy", async () => {
    const component = mount(CommandPalette, {
      target: document.body,
    });

    await tick();

    unmount(component);

    expect(mockSearchStore.clear).toHaveBeenCalledOnce();
    expect(mockSearchStore.resetSort).toHaveBeenCalledOnce();
  });

  it("copies canonical session_id to clipboard, not stripped display ID", async () => {
    // Prefixed ID: stripIdPrefix("codex:abc123def456", "codex") → "abc123def456"
    // Display shows first 8 chars: "abc123de"
    // Copy must use the full canonical "codex:abc123def456"
    mockSearchStore.results = [
      makeSearchResult({
        session_id: "codex:abc123def456",
        project: "test-proj",
        ordinal: 0,
        snippet: "some matching text",
      }),
    ];

    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery("abc");

    const badge = await tickUntil(".item-id");
    expect(badge.textContent?.trim()).toBe("abc123de");

    badge.click();
    await tick();

    expect(mockCopyToClipboard).toHaveBeenCalledWith("codex:abc123def456");

    unmount(component);
  });

  it("omits relative-time segment when timestamp is empty", async () => {
    mockSearchStore.results = [
      makeSearchResult({
        session_id: "codex:emptytime123",
        project: "my-proj",
        ordinal: 0,
        timestamp: "",
        snippet: "some text",
      }),
    ];

    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery("abc");

    const meta = await tickUntil(".item-meta");
    // Should show project but no " · <time>" segment.
    expect(meta.textContent?.trim()).toBe("my-proj");

    unmount(component);
  });

  it("name-only result (ordinal === -1) routes and clears selection without scrolling", async () => {
    mockSearchStore.results = [
      makeSearchResult({
        session_id: "claude:nameonly123",
        project: "proj-a",
        agent: "claude",
        name: "nameonly match",
        ordinal: -1,
        snippet: "",
      }),
    ];

    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery("nameonly");

    const item = await tickUntil(".palette-item");
    item.click();
    await tick();

    expect(mockRouter.navigateToSession).toHaveBeenCalledWith("claude:nameonly123");
    expect(mockUi.scrollToOrdinal).not.toHaveBeenCalled();
    expect(mockUi.clearScrollState).toHaveBeenCalled();

    unmount(component);
  });

  it("does not reselect typed text after each keystroke (#795)", async () => {
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    const input = document.querySelector<HTMLInputElement>(".palette-input")!;

    function typeChar(char: string) {
      const start = input.selectionStart ?? input.value.length;
      const end = input.selectionEnd ?? input.value.length;
      input.value = input.value.slice(0, start) + char + input.value.slice(end);
      input.setSelectionRange(start + 1, start + 1);
      input.dispatchEvent(
        new InputEvent("input", { bubbles: true, inputType: "insertText", data: char }),
      );
    }

    typeChar("a");
    await tick();

    expect(input.value).toBe("a");
    expect(input.selectionStart).toBe(1);
    expect(input.selectionEnd).toBe(1);

    typeChar("b");
    await tick();

    expect(input.value).toBe("ab");
    expect(input.selectionStart).toBe(2);
    expect(input.selectionEnd).toBe(2);

    unmount(component);
  });

  it.each([
    { query: "消融", searches: true },
    { query: "検索", searches: true },
    { query: "검색", searches: true },
    { query: "a消", searches: true },
    { query: "abc", searches: true },
    { query: "消", searches: false },
    { query: "ab", searches: false },
  ])("sends $query to the server: $searches", async ({ query, searches }) => {
    mockSearchStore.results = [makeSearchResult({ snippet: "server hit" })];
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery(query);

    if (searches) {
      expect(mockSearchStore.search).toHaveBeenCalledWith(query, "");
      expect(mockSearchStore.clear).not.toHaveBeenCalled();
    } else {
      expect(mockSearchStore.search).not.toHaveBeenCalled();
      expect(mockSearchStore.clear).toHaveBeenCalled();
    }
    const snippets = Array.from(document.querySelectorAll(".item-snippet")).map((el) =>
      el.textContent?.trim(),
    );
    expect(snippets.includes("server hit")).toBe(searches);

    unmount(component);
  });

  it("filters recent sessions locally for a single CJK character", async () => {
    mockSessions.sessions = [
      { ...makeSession("s1", "codex"), first_message: "消融实验的结果" },
      { ...makeSession("s2", "codex"), first_message: "unrelated" },
    ];
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery("消");

    const items = document.querySelectorAll(".palette-item");
    expect(items).toHaveLength(1);
    expect(items[0]?.textContent).toContain("消融实验的结果");

    unmount(component);
  });

  it("search result click navigates to the session route", async () => {
    mockSearchStore.results = [makeSearchResult()];

    const component = mount(CommandPalette, { target: document.body });
    await tick();

    await enterSearchQuery();

    const item = await tickUntil(".palette-item");
    item.click();
    await tick();

    expect(mockRouter.navigateToSession).toHaveBeenCalledWith("codex:search123");
    expect(mockUi.scrollToOrdinal).toHaveBeenCalledWith(7, "codex:search123");

    unmount(component);
  });

  it("recent result click routes to the session and closes the palette", async () => {
    const component = mount(CommandPalette, { target: document.body });
    await tick();

    const item = await tickUntil(".palette-item");
    item.click();
    await tick();

    expect(mockRouter.navigateToSession).toHaveBeenCalledWith("s1");
    expect(mockUi.activeModal).toBeNull();

    unmount(component);
  });

  it("renders loading before error, empty state, and results", async () => {
    mockSearchStore.isSearching = true;
    mockSearchStore.error = { detail: "backend detail", kind: "generic" };
    mockSearchStore.results = [makeSearchResult()];
    const component = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();

    expect(document.querySelector(".palette-empty")?.textContent).toContain("Searching");
    expect(document.querySelector(".palette-error")).toBeNull();
    expect(document.querySelector(".palette-item")).toBeNull();

    unmount(component);
  });

  it("renders the localized error heading and exact backend detail before results", async () => {
    mockSearchStore.error = {
      detail: "Run agentsview embeddings build --full-rebuild",
      kind: "generic",
    };
    mockSearchStore.results = [makeSearchResult()];
    const component = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();

    const error = document.querySelector(".palette-error");
    expect(error?.querySelector("strong")?.textContent).toBe("Search unavailable");
    expect(error?.querySelector("span")?.textContent).toBe(
      "Run agentsview embeddings build --full-rebuild",
    );
    expect(document.querySelector(".palette-item")).toBeNull();

    unmount(component);
  });

  it("renders localized fallback copy for an error without string detail", async () => {
    mockSearchStore.error = { detail: null, kind: "generic" };
    const component = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();

    const error = document.querySelector(".palette-error");
    expect(error?.querySelector("strong")?.textContent).toBe("Search unavailable");
    expect(error?.querySelector("span")?.textContent).toBe("Search failed. Please try again.");

    unmount(component);
  });

  it("shows the inherited project and can search all projects without changing the sidebar", async () => {
    mockSessions.filters.project = "proj-a";
    const component = mount(CommandPalette, { target: document.body });
    try {
      await enterSearchQuery("configuration");
      expect(mockSearchStore.search).toHaveBeenLastCalledWith("configuration", "proj-a");
      const trigger = Array.from(
        document.querySelectorAll<HTMLButtonElement>(".palette-controls button"),
      ).find((button) => button.textContent?.trim() === "proj-a")!;
      expect(trigger).toBeDefined();
      trigger.click();
      await tick();
      const all = Array.from(document.querySelectorAll<HTMLElement>("[role='option']")).find(
        (option) => option.textContent?.trim() === "All Projects",
      )!;
      all.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
      await tick();
      expect(mockSearchStore.search).toHaveBeenLastCalledWith("configuration", "");
      expect(mockSessions.filters.project).toBe("proj-a");
      await enterSearchQuery("configuration details");
      expect(mockSearchStore.search).toHaveBeenLastCalledWith("configuration details", "");
      expect(document.querySelector(".palette-controls")?.textContent).toContain("All Projects");
    } finally {
      await unmount(component);
    }
  });

  it("renders empty and result states when no higher-priority state applies", async () => {
    const empty = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();
    expect(document.querySelector(".palette-empty")?.textContent).toContain("No results");
    unmount(empty);

    mockSearchStore.results = [makeSearchResult({ snippet: "visible result" })];
    const results = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();
    expect(document.querySelector(".item-snippet")?.textContent).toBe("visible result");
    expect(document.querySelector(".palette-empty")).toBeNull();
    unmount(results);
  });

  it("preserves sanitized full-text mark highlighting", async () => {
    mockSearchStore.results = [
      makeSearchResult({
        snippet: "before <mark>needle</mark> after",
        snippetFormat: "highlighted-html",
      }),
    ];
    const component = mount(CommandPalette, { target: document.body });
    await enterSearchQuery();

    const mark = document.querySelector(".item-snippet mark");
    expect(mark?.textContent).toBe("needle");

    unmount(component);
  });
});
