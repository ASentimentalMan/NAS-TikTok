import router from "@/router";

const apiBase = () => {
  const configured = (window as any).__APP_CONFIG__?.API_BASE;
  return (configured || "/api").replace(/\/$/, "");
};

const redirectToLogin = () => {
  localStorage.removeItem("token");
  if (router.currentRoute.value.name !== "login") {
    void router.replace({ name: "login", query: { redirect: router.currentRoute.value.fullPath } });
  }
};

const requestJSON = async (path: string, options: RequestInit = {}) => {
  const response = await fetch(`${apiBase()}${path}`, {
    ...options,
    credentials: "include",
    headers: { ...options.headers },
  });
  const result = await response.json().catch(() => ({}));
  if (response.status === 401) redirectToLogin();
  if (!response.ok) {
    throw new Error(result.message || `请求失败 (${response.status})`);
  }
  return result;
};

export const captcha = () => requestJSON("/captcha");

export const login = (data: Record<string, unknown>) =>
  requestJSON("/logIn", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });

export const logout = () => requestJSON("/logout", { method: "POST" });

export const checkSession = async (): Promise<
  "authenticated" | "unauthenticated" | "unavailable"
> => {
  try {
    const response = await fetch(`${apiBase()}/session`, { credentials: "include" });
    if (response.status === 401) return "unauthenticated";
    return response.ok ? "authenticated" : "unavailable";
  } catch {
    return "unavailable";
  }
};

const fetchMediaList = async (type: "image" | "video", signal?: AbortSignal) => {
  const result = await requestJSON(`/list?type=${type}`, { signal });
  result.data = result.data.map((url: string) => ({ id: url, url }));
  return result;
};

export const fetchVideoList = (signal?: AbortSignal) =>
  fetchMediaList("video", signal);
export const fetchImageList = (signal?: AbortSignal) =>
  fetchMediaList("image", signal);
