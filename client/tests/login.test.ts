import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { apiMock, routerMock, credentialMock } = vi.hoisted(() => ({
  apiMock: { captcha: vi.fn(), login: vi.fn() },
  routerMock: { replace: vi.fn() },
  credentialMock: {
    clearSavedCredentials: vi.fn(),
    credentialOperationVersion: vi.fn(() => 1),
    credentialPersistenceNotice: { value: "" },
    getCredentialPreference: vi.fn((): boolean | null => null),
    isHttpsOrigin: vi.fn(() => true),
    loadSavedCredentials: vi.fn(),
    persistSuccessfulLogin: vi.fn(),
    setCredentialPreference: vi.fn(() => ({
      version: 1,
      markerWritten: false,
      preferenceWritten: true,
    })),
  },
}));

vi.mock("@/apis/index", () => apiMock);
vi.mock("@/router", () => ({ default: routerMock }));
vi.mock("@/utils/saved-credentials", () => credentialMock);

import LoginView from "@/views/login/index.vue";

describe("login saved credentials", () => {
  beforeEach(() => {
    localStorage.clear();
    apiMock.captcha.mockReset().mockResolvedValue({ id: "captcha", data: "" });
    apiMock.login.mockReset();
    routerMock.replace.mockReset();
    credentialMock.clearSavedCredentials.mockReset().mockResolvedValue({
      complete: true,
      futureAutofillBlocked: true,
      versionIsCurrent: true,
    });
    credentialMock.credentialOperationVersion.mockReset().mockReturnValue(1);
    credentialMock.credentialPersistenceNotice.value = "";
    credentialMock.loadSavedCredentials.mockReset().mockResolvedValue({ status: "empty" });
    credentialMock.persistSuccessfulLogin.mockReset().mockResolvedValue({ status: "saved" });
    credentialMock.getCredentialPreference.mockReset().mockReturnValue(null);
    credentialMock.isHttpsOrigin.mockReset().mockReturnValue(true);
    credentialMock.setCredentialPreference.mockReset().mockReturnValue({
      version: 1,
      markerWritten: false,
      preferenceWritten: true,
    });
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("keeps the legacy account, removes its plaintext password, and does not opt in", async () => {
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
    expect((wrapper.findAll("input")[0].element as HTMLInputElement).value).toBe("legacy-user");
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    expect((wrapper.find('input[type="password"]').element as HTMLInputElement).value).toBe("");

    apiMock.login.mockRejectedValueOnce(new Error("offline"));
    await wrapper.find('input[type="password"]').setValue("new-password");
    await wrapper.findAll("input")[2].setValue("123");
    await wrapper.get(".button").trigger("click");
    await flushPromises();

    expect(localStorage.getItem("savedLogin")).toBe(JSON.stringify({ account: "legacy-user" }));
    expect(credentialMock.persistSuccessfulLogin).not.toHaveBeenCalled();
    expect(apiMock.captcha).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it("keeps the password visible and reports a save failure after login success", async () => {
    apiMock.login.mockResolvedValueOnce({ ok: true });
    credentialMock.persistSuccessfulLogin.mockResolvedValueOnce({
      status: "failed",
      notice: "登录成功，但本机未能安全保存账号密码。",
    });

    const wrapper = mount(LoginView);
    await flushPromises();
    await wrapper.findAll("input")[0].setValue("user");
    await wrapper.find('input[type="password"]').setValue("secret");
    await wrapper.findAll("input")[2].setValue("123");
    await wrapper.find('input[type="checkbox"]').setValue(true);
    await wrapper.get(".button").trigger("click");
    await flushPromises();

    expect(routerMock.replace).not.toHaveBeenCalled();
    expect(credentialMock.persistSuccessfulLogin).toHaveBeenCalledWith("user", "secret", 1);
    expect(apiMock.captcha).toHaveBeenCalledTimes(1);
    expect((wrapper.find('input[type="password"]').element as HTMLInputElement).value).toBe("secret");
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true);
    expect(wrapper.text()).toContain("登录成功，但本机未能安全保存账号密码。");
    expect(credentialMock.credentialPersistenceNotice.value).toBe(
      "登录成功，但本机未能安全保存账号密码。"
    );
    wrapper.unmount();
  });

  it("reports a saved result warning after navigation", async () => {
    apiMock.login.mockResolvedValueOnce({ ok: true });
    credentialMock.persistSuccessfulLogin.mockResolvedValueOnce({
      status: "saved",
      notice: "账号偏好无法保存在浏览器中。",
    });

    const wrapper = mount(LoginView);
    await flushPromises();
    await wrapper.findAll("input")[0].setValue("user");
    await wrapper.find('input[type="password"]').setValue("secret");
    await wrapper.findAll("input")[2].setValue("123");
    await wrapper.find('input[type="checkbox"]').setValue(true);
    await wrapper.get(".button").trigger("click");
    await flushPromises();

    expect(routerMock.replace).toHaveBeenCalledWith({ path: "/" });
    expect(credentialMock.credentialPersistenceNotice.value).toBe(
      "账号偏好无法保存在浏览器中。"
    );
    expect(apiMock.captcha).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("restores the remembered checkbox even before a saved password is available", async () => {
    credentialMock.getCredentialPreference.mockReturnValueOnce(true);

    const wrapper = mount(LoginView);
    await flushPromises();

    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true);
    expect((wrapper.find('input[type="password"]').element as HTMLInputElement).value).toBe("");
    expect(credentialMock.setCredentialPreference).toHaveBeenCalledWith(true);

    wrapper.unmount();
  });

  it("hides the remember option and skips credential loading on HTTP", async () => {
    credentialMock.isHttpsOrigin.mockReturnValueOnce(false);

    const wrapper = mount(LoginView);
    await flushPromises();

    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false);
    expect(credentialMock.getCredentialPreference).not.toHaveBeenCalled();
    expect(credentialMock.loadSavedCredentials).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it.each([
    { initially: false, finally: true, saves: true },
    { initially: true, finally: false, saves: false },
  ])("uses the latest remember choice while login is pending: $initially -> $finally", async ({ initially, finally: finallyRemember, saves }) => {
    let resolveLogin!: (value: { ok: true }) => void;
    apiMock.login.mockImplementationOnce(() => new Promise((resolve) => { resolveLogin = resolve; }));

    const wrapper = mount(LoginView);
    await flushPromises();
    await wrapper.findAll("input")[0].setValue("user");
    await wrapper.find('input[type="password"]').setValue("secret");
    await wrapper.findAll("input")[2].setValue("123");
    const checkbox = wrapper.find('input[type="checkbox"]');
    if (initially) await checkbox.setValue(true);

    await wrapper.get(".button").trigger("click");
    await checkbox.setValue(finallyRemember);
    resolveLogin({ ok: true });
    await flushPromises();

    expect(credentialMock.persistSuccessfulLogin).toHaveBeenCalledTimes(saves ? 1 : 0);
    expect(routerMock.replace).toHaveBeenCalledWith({ path: "/" });
    wrapper.unmount();
  });

  it("keeps the typed password and ignores a pending decrypt after remember is cancelled", async () => {
    let resolveRead!: (value: { status: string; account?: string; password?: string }) => void;
    credentialMock.loadSavedCredentials.mockImplementationOnce(
      () => new Promise((resolve) => { resolveRead = resolve; })
    );

    const wrapper = mount(LoginView);
    await wrapper.find('input[type="password"]').setValue("typed-secret");
    await wrapper.find('input[type="checkbox"]').setValue(true);
    await wrapper.find('input[type="checkbox"]').setValue(false);
    resolveRead({ status: "loaded", account: "old-user", password: "old-secret" });
    await flushPromises();

    expect((wrapper.findAll("input")[0].element as HTMLInputElement).value).toBe("");
    expect((wrapper.find('input[type="password"]').element as HTMLInputElement).value).toBe("typed-secret");
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    expect(credentialMock.clearSavedCredentials).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});
