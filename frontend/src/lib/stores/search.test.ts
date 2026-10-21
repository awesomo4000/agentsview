import { afterEach, beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { SearchService } from "../api/generated/index.js";
import { ApiError } from "../api/runtime.js";
import type { SearchResponse } from "../api/generated/index.js";
import { SEARCH_MODE_STORAGE_KEY, createSearchStore } from "./search.svelte.js";

vi.mock("../api/generated/index.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/generated/index.js")>();
  return {
    ...actual,
    SearchService: {
      getApiV1Search: vi.fn(),
      getApiV1SearchContent: vi.fn(),
    },
  };
});

const searchService = vi.mocked(SearchService);
const DEBOUNCE_MS = 300;

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => values.set(key, value)),
  };
}

function fullTextResponse(query: string, count = 1): SearchResponse {
  return {
    query,
    count,
    next: 0,
    results: Array.from({ length: count }, (_, index) => ({
      session_id: `full-${index}`,
      project: "alpha",
      machine: "node-a",
      agent: "codex",
      name: `Result ${index}`,
      ordinal: index + 2,
      session_ended_at: `2026-07-0${index + 1}T12:00:00Z`,
      snippet: `before <mark>result ${index}</mark> after`,
      rank: index + 0.25,
    })),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function generatedApiError(status: number, detail: string): ApiError {
  return new ApiError(status, detail);
}

async function runDebounce() {
  await vi.advanceTimersByTimeAsync(DEBOUNCE_MS);
  await Promise.resolve();
}

async function flushMicrotasks(ticks = 5) {
  for (let index = 0; index < ticks; index += 1) await Promise.resolve();
}

describe("SearchStore", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each([
    [undefined, "fulltext"],
    ["not-a-mode", "fulltext"],
    ["semantic", "fulltext"],
    ["hybrid", "fulltext"],
  ] as const)("loads persisted mode %s as %s", (stored, expected) => {
    const storage = memoryStorage(
      stored === undefined ? {} : { [SEARCH_MODE_STORAGE_KEY]: stored },
    );

    expect(createSearchStore(storage).mode).toBe(expected);
  });

  it("defensively handles unavailable storage", () => {
    const throwingStorage = {
      getItem: vi.fn(() => {
        throw new DOMException("denied", "SecurityError");
      }),
      setItem: vi.fn(() => {
        throw new DOMException("denied", "SecurityError");
      }),
    };
    const store = createSearchStore(throwingStorage);

    expect(store.mode).toBe("fulltext");
    expect(() => store.setMode("semantic")).not.toThrow();
    expect(store.mode).toBe("fulltext");
  });

  it("searches with FTS despite a saved semantic preference", async () => {
    const store = createSearchStore(memoryStorage({ [SEARCH_MODE_STORAGE_KEY]: "semantic" }));
    searchService.getApiV1Search.mockResolvedValue({
      results: [],
      count: 0,
      next: 0,
      query: "authentication timeout",
    });
    store.search("authentication timeout");
    await vi.advanceTimersByTimeAsync(300);
    expect(searchService.getApiV1Search).toHaveBeenCalledWith(
      expect.objectContaining({ q: "authentication timeout", sort: "relevance" }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(searchService.getApiV1SearchContent).not.toHaveBeenCalled();
  });

  it("calls full-text search with the exact request and normalizes its result", async () => {
    const store = createSearchStore(memoryStorage());
    searchService.getApiV1Search.mockResolvedValueOnce(fullTextResponse("needle"));

    store.search("needle", "alpha");
    await runDebounce();

    expect(searchService.getApiV1Search).toHaveBeenCalledWith(
      {
        q: "needle",
        project: "alpha",
        limit: 30,
        sort: "relevance",
      },
      expect.objectContaining({ signal: expect.anything() }),
    );
    expect(searchService.getApiV1SearchContent).not.toHaveBeenCalled();
    expect(store.results).toEqual([
      {
        session_id: "full-0",
        project: "alpha",
        agent: "codex",
        name: "Result 0",
        ordinal: 2,
        timestamp: "2026-07-01T12:00:00Z",
        snippet: "before <mark>result 0</mark> after",
        rank: 0.25,
        snippetFormat: "highlighted-html",
      },
    ]);
  });

  it("keeps a full-text 501 generic: setup guidance only applies to semantic modes", async () => {
    const store = createSearchStore(memoryStorage());
    searchService.getApiV1Search.mockRejectedValueOnce(generatedApiError(501, "not implemented"));

    store.search("fulltext failure");
    await runDebounce();

    expect(store.error).toEqual({ detail: "not implemented", kind: "generic" });
  });

  it("clears an earlier error when a new request starts and on clear", async () => {
    const store = createSearchStore(memoryStorage());
    searchService.getApiV1Search.mockRejectedValueOnce(new Error("network offline"));
    store.search("first");
    await runDebounce();
    expect(store.error).toEqual({
      detail: "network offline",
      kind: "generic",
    });

    const pending = deferred<SearchResponse>();
    searchService.getApiV1Search.mockReturnValueOnce(pending.promise as never);
    store.search("second");
    await runDebounce();
    expect(store.error).toBeNull();
    expect(store.isSearching).toBe(true);

    store.clear();
    expect(store.error).toBeNull();
    expect(store.isSearching).toBe(false);
  });

  it("keeps unknown failures detail-less for localized presentation", async () => {
    const store = createSearchStore(memoryStorage());
    searchService.getApiV1Search.mockRejectedValueOnce({ reason: "offline" });

    store.search("unknown failure");
    await runDebounce();

    expect(store.error).toEqual({ detail: null, kind: "generic" });
    expect(store.isSearching).toBe(false);
  });

  it("clear cancels a generated request so it cannot write later", async () => {
    const store = createSearchStore(memoryStorage());
    let completeRequest!: (value: SearchResponse) => void;
    let wasCancelled = false;
    searchService.getApiV1Search.mockImplementationOnce(
      (_params, options) =>
        new Promise<SearchResponse>((resolve, reject) => {
          completeRequest = resolve;
          options?.signal?.addEventListener(
            "abort",
            () => {
              reject(new DOMException("Aborted", "AbortError"));
              wasCancelled = true;
            },
            { once: true },
          );
        }) as never,
    );

    store.search("cancel me");
    await runDebounce();
    store.clear();
    completeRequest(fullTextResponse("cancel me", 2));
    await flushMicrotasks();

    expect(wasCancelled).toBe(true);
    expect(store.results).toEqual([]);
    expect(store.query).toBe("");
    expect(store.isSearching).toBe(false);
  });
});
