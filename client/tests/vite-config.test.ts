import { describe, expect, it } from "vitest";

import { createApiProxyRewrite } from "../vite.config";

describe("Vite development API proxy", () => {
  it.each([
    ["/", "/api/captcha", "/api/captcha"],
    ["/photos/", "/api/captcha?source=login", "/photos/api/captcha?source=login"],
  ])("maps API requests under base %s", (baseUrl, requestPath, expectedPath) => {
    expect(createApiProxyRewrite(baseUrl)(requestPath)).toBe(expectedPath);
  });
});
