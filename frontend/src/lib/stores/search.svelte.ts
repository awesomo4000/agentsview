import { debounce } from "@kenn-io/kit-ui";
import { SearchService } from "../api/generated/index.js";
import { ApiError, isAbortError } from "../api/runtime.js";
import type { DbSearchResult as SearchResult } from "../api/generated/index.js";
import { resolveRange, type RangeSelection } from "../components/shared/rangeSelection.js";

export type SearchMode = "fulltext" | "semantic" | "hybrid";
export type SearchSort = "relevance" | "recency";
export type SnippetFormat = "highlighted-html" | "plain-text";

export interface PaletteSearchResult {
  session_id: string;
  project: string;
  agent: string;
  name?: string;
  ordinal: number;
  timestamp: string;
  snippet: string;
  rank: number;
  snippetFormat: SnippetFormat;
}

export interface SearchFailure {
  detail: string | null;
  kind: "generic" | "timeout" | "semantic-unavailable";
}

type SearchModeStorage = Pick<Storage, "getItem" | "setItem">;

export const SEARCH_MODE_STORAGE_KEY = "agentsview-search-mode";
const SEARCH_DEBOUNCE_MS = 300;
const PALETTE_RESULT_LIMIT = 30;
const SEARCH_MODES: readonly SearchMode[] = ["fulltext"];

function availableStorage(): SearchModeStorage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

function loadMode(storage: SearchModeStorage | null): SearchMode {
  try {
    const value = storage?.getItem(SEARCH_MODE_STORAGE_KEY);
    return SEARCH_MODES.includes(value as SearchMode) ? (value as SearchMode) : "fulltext";
  } catch {
    return "fulltext";
  }
}

function normalizeFullText(results: SearchResult[]): PaletteSearchResult[] {
  return results.map((result) => ({
    session_id: result.session_id,
    project: result.project,
    agent: result.agent,
    name: result.name || undefined,
    ordinal: result.ordinal,
    timestamp: result.session_ended_at,
    snippet: result.snippet,
    rank: result.rank,
    snippetFormat: "highlighted-html",
  }));
}

function errorDetail(error: unknown): string | null {
  if (error instanceof ApiError || error instanceof Error) {
    return error.message || null;
  }
  return null;
}

function isTimeoutDetail(detail: string | null): boolean {
  return detail !== null && /(?:timed out|timeout|deadline exceeded)/i.test(detail);
}

// failureKind classifies semantic/hybrid failures the palette can act on.
// A 501 means semantic search is not serving on this daemon (vector config
// disabled, or no index built yet) — the palette renders setup guidance for
// it instead of the raw backend error string.
function failureKind(
  mode: SearchMode,
  error: unknown,
  detail: string | null,
): SearchFailure["kind"] {
  if (mode === "fulltext") return "generic";
  if (error instanceof ApiError && error.status === 501) {
    return "semantic-unavailable";
  }
  return isTimeoutDetail(detail) ? "timeout" : "generic";
}

export class SearchStore {
  query: string = $state("");
  project: string = $state("");
  sort: SearchSort = $state("relevance");
  mode: SearchMode = $state("fulltext");
  range: RangeSelection = $state({ mode: "relative", days: 0 });
  results: PaletteSearchResult[] = $state([]);
  isSearching: boolean = $state(false);
  error: SearchFailure | null = $state(null);

  private storage: SearchModeStorage | null;
  private abortController: AbortController | null = null;
  private requestVersion = 0;

  private debouncedSearch = debounce((query: string, project: string) => {
    void this.executeSearch(query, project);
  }, SEARCH_DEBOUNCE_MS);

  constructor(storage: SearchModeStorage | null = availableStorage()) {
    this.storage = storage;
    this.mode = loadMode(storage);
  }

  search(query: string, project?: string) {
    this.query = query;
    if (project !== undefined) this.project = project;

    this.cancelInFlight();
    if (!query.trim()) {
      this.debouncedSearch.cancel();
      this.results = [];
      this.error = null;
      this.isSearching = false;
      return;
    }

    this.debouncedSearch(query, this.project);
  }

  setMode(_mode: SearchMode) {
    const mode: SearchMode = "fulltext";
    if (mode === this.mode) return;
    this.mode = mode;
    try {
      this.storage?.setItem(SEARCH_MODE_STORAGE_KEY, mode);
    } catch {
      // Storage can be disabled; the in-memory preference still applies.
    }

    this.debouncedSearch.cancel();
    this.cancelInFlight();
    if (this.query.trim()) {
      void this.executeSearch(this.query, this.project);
    }
  }

  setSort(sort: SearchSort) {
    this.sort = sort;
    if (this.query.trim()) {
      this.debouncedSearch.cancel();
      this.cancelInFlight();
      void this.executeSearch(this.query, this.project);
    }
  }

  retry() {
    if (!this.query.trim() || !this.error) return;
    this.debouncedSearch.cancel();
    this.cancelInFlight();
    void this.executeSearch(this.query, this.project);
  }

  setRange(range: RangeSelection) {
    this.range = range;
    this.debouncedSearch.cancel();
    this.cancelInFlight();
    if (this.query.trim()) {
      void this.executeSearch(this.query, this.project);
    }
  }

  resetRange() {
    this.range = { mode: "relative", days: 0 };
  }

  clear() {
    this.query = "";
    this.results = [];
    this.error = null;
    this.isSearching = false;
    this.debouncedSearch.cancel();
    this.cancelInFlight();
  }

  resetSort() {
    this.sort = "relevance";
  }

  private cancelInFlight() {
    this.requestVersion += 1;
    this.abortController?.abort();
    this.abortController = null;
  }

  private async executeSearch(query: string, project: string) {
    this.cancelInFlight();
    const requestVersion = this.requestVersion;
    const controller = new AbortController();
    const { signal } = controller;
    this.abortController = controller;
    this.isSearching = true;
    this.error = null;
    const mode = this.mode;
    // All time must omit both bounds, rather than use the picker's fallback
    // start date when the earliest archived session is unknown.
    const range =
      this.range.mode === "relative" && this.range.days === 0 ? null : resolveRange(this.range);
    const dates = range ? { date_from: range.from, date_to: range.to } : {};

    try {
      let results: PaletteSearchResult[];
      const response = await SearchService.getApiV1Search(
        {
          q: query,
          project: project || undefined,
          limit: PALETTE_RESULT_LIMIT,
          sort: this.sort,
          ...dates,
        },
        { signal },
      );
      results = normalizeFullText(response.results ?? []);

      if (requestVersion !== this.requestVersion || signal.aborted) return;
      this.results = results;
      this.error = null;
    } catch (error: unknown) {
      if (requestVersion !== this.requestVersion || signal.aborted || isAbortError(error)) {
        return;
      }
      this.results = [];
      const detail = errorDetail(error);
      this.error = { detail, kind: failureKind(mode, error, detail) };
    } finally {
      if (requestVersion === this.requestVersion && !signal.aborted) {
        this.isSearching = false;
        this.abortController = null;
      }
    }
  }
}

export function createSearchStore(
  storage: SearchModeStorage | null = availableStorage(),
): SearchStore {
  return new SearchStore(storage);
}

export const searchStore = createSearchStore();
