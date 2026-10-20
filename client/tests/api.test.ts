import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { routerMock } = vi.hoisted(() => ({
  routerMock: {
    currentRoute: { value: { name: "home", fullPath: "/private" } },
    replace: vi.fn(),
  },
}));

vi.mock("@/router", () => ({ default: routerMock }));

import { captcha, checkSession } from "@/apis/index";

const response = (status: number, body: unknown = {}) =>
  ({
    status,
    ok: status >= 200 && status < 300,
    json: async () => body,
  }) as Response;

describe("cookie-auth API requests", () => {
  beforeEach(() => {
    localStorage.clear();
    routerMock.currentRoute.value = { name: "home", fullPath: "/private" };
    routerMock.replace.mockReset();
    vi.stubGlobal("fetch", vi.fn());
    (window as any).__APP_CONFIG__ = { API_BASE: "/photos/api/" };
  });

  afterEach(() => {
    localStorage.clear();
    delete (window as any).__APP_CONFIG__;
    vi.unstubAllGlobals();
  });

  it("uses the runtime API base and sends the session cookie", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValue(response(200, { id: "challenge" }));

    await expect(captcha()).resolves.toEqual({ id: "challenge" });
    expect(fetchMock).toHaveBeenCalledWith("/photos/api/captcha", {
      credentials: "include",
      headers: {},
    });
  });

  it.each([
    [200, "authenticated"],
    [401, "unauthenticated"],
    [503, "unavailable"],
  ] as const)("maps session status %i to %s without redirecting", async (status, expected) => {
    vi.mocked(fetch).mockResolvedValue(response(status));

    await expect(checkSession()).resolves.toBe(expected);
    expect(routerMock.replace).not.toHaveBeenCalled();
  });

  it("redirects and clears the legacy token on API 401", async () => {
    localStorage.setItem("token", "stale-token");
    vi.mocked(fetch).mockResolvedValue(response(401, { message: "需要登录" }));

    await expect(captcha()).rejects.toMatchObject({
      message: "需要登录",
      status: 401,
      retryable: false,
    });

    expect(localStorage.getItem("token")).toBeNull();
    expect(routerMock.replace).toHaveBeenCalledWith({
      name: "login",
      query: { redirect: "/private" },
    });
  });

  it.each([429, 500, 503])("marks API %i as retryable", async (status) => {
    vi.mocked(fetch).mockResolvedValue(response(status, { message: "temporary failure" }));

    await expect(captcha()).rejects.toMatchObject({ status, retryable: true });
  });

  it("does not mark permanent client errors as retryable", async () => {
    vi.mocked(fetch).mockResolvedValue(response(403, { message: "forbidden" }));

    await expect(captcha()).rejects.toMatchObject({ status: 403, retryable: false });
  });

  it("keeps the local session marker on network errors", async () => {
    localStorage.setItem("token", "legacy-marker");
    vi.mocked(fetch).mockRejectedValue(new TypeError("offline"));

    await expect(checkSession()).resolves.toBe("unavailable");
    await expect(captcha()).rejects.toMatchObject({ message: "offline", retryable: true });

    expect(localStorage.getItem("token")).toBe("legacy-marker");
    expect(routerMock.replace).not.toHaveBeenCalled();
  });
});
