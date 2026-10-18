import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import { getApiV1Version } from "./generated/metadata/metadata.js";
import {
  ApiError,
  orvalFetch,
  getServerUrl,
  responseTimingOf,
  SERVER_URL_CHANGE_EVENT,
  setAuthToken,
  setServerUrl,
} from "./runtime.js";
import { requestUrl } from "../test-support/request-url.js";

describe("orvalFetch", () => {
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("ignores stale remote selection and rejects new remote servers", () => {
localStorage.setItem("agentsview-server-url", "https://example.invalid");
 expect(getServerUrl()).toBe("");
 expect(() => setServerUrl("https://example.invalid")).toThrow();
 expect(getServerUrl()).toBe("");
});

  it("records when a JSON response was sent, answered, and read", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        vi.advanceTimersByTime(40);
        const response = new Response('{"ok":true}', {
          headers: { "Content-Type": "application/json" },
        });
        const text = response.text.bind(response);
        response.text = async () => {
          vi.advanceTimersByTime(15);
          return text();
        };
        return response;
      }),
    );
    const before = performance.now();

    const data = await orvalFetch<{ ok: boolean }>("/api/v1/usage/summary", {});

    expect(responseTimingOf(data)).toEqual({
      sentAt: before,
      headersAt: before + 40,
      bodyAt: before + 55,
    });
  });

  it("keeps generated requests local despite a saved remote URL", async () => {
    localStorage.setItem("agentsview-server-url", "https://example.test");
    setAuthToken("secret");
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response("{}", {
          headers: { "Content-Type": "application/json" },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await orvalFetch("/api/v1/usage/summary", {});

    const [input, init] = fetchMock.mock.calls[0]!;
    expect(requestUrl(input)).toBe("/api/v1/usage/summary");
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer secret");
  });
  it("refuses remote candidate requests before fetch", async () => {
const fetchMock = vi.fn(); vi.stubGlobal("fetch",fetchMock);
 await expect(getApiV1Version({baseUrl:"https://candidate.example.invalid",headers:{Authorization:"Bearer synthetic"}})).rejects.toThrow();
 expect(fetchMock).not.toHaveBeenCalled();
});
});

describe("API errors", () => {
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it("normalizes generated API error bodies and codes", async () => {
    localStorage.setItem("agentsview-server-url", "http://localhost");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "unknown_project_key",
            error: "unknown project key",
          }),
          { status: 400, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    await expect(orvalFetch("/api/v1/usage/summary", {})).rejects.toMatchObject({
      name: "ApiError",
      status: 400,
      code: "unknown_project_key",
      message: "unknown project key",
    } satisfies Partial<ApiError>);
  });
});
