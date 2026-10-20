import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { apiMock, routerMock } = vi.hoisted(() => ({
  apiMock: { captcha: vi.fn(), login: vi.fn() },
  routerMock: { replace: vi.fn() },
}));

vi.mock("@/apis/index", () => apiMock);
vi.mock("@/router", () => ({ default: routerMock }));

import LoginView from "@/views/login/index.vue";

describe("login saved-account migration", () => {
  beforeEach(() => {
    localStorage.clear();
    apiMock.captcha.mockReset().mockResolvedValue({ id: "captcha", data: "" });
    apiMock.login.mockReset();
    routerMock.replace.mockReset();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("removes passwords from legacy savedLogin and does not restore one after failure", async () => {
    localStorage.setItem("token", "stale-token");
    localStorage.setItem(
      "savedLogin",
      JSON.stringify({ account: "legacy-user", password: "old-password" })
    );

    const wrapper = mount(LoginView);
    await flushPromises();

    expect(localStorage.getItem("token")).toBeNull();
    expect(localStorage.getItem("savedLogin")).toBe(
      JSON.stringify({ account: "legacy-user" })
    );
    expect((wrapper.find("input").element as HTMLInputElement).value).toBe("legacy-user");

    apiMock.login.mockRejectedValueOnce(new Error("offline"));
    await wrapper.find('input[type="password"]').setValue("new-password");
    await wrapper.findAll("input")[2].setValue("123");
    await wrapper.get(".button").trigger("click");
    await flushPromises();

    const savedLogin = localStorage.getItem("savedLogin") || "";
    expect(savedLogin).toBe(JSON.stringify({ account: "legacy-user" }));
    expect(savedLogin).not.toContain("password");
    expect(savedLogin).not.toContain("new-password");
    wrapper.unmount();
  });
});
