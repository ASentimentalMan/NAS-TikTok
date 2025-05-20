import router from "@/router";
import { generateUUID } from "@/utils/tool.util";

export const captcha = async () => {
  const response = await fetch(
    `${(window as any).__APP_CONFIG__.VITE_API_URL}:${
      (window as any).__APP_CONFIG__.VITE_API_PORT
    }/api/captcha`
  );
  const result = await response.json();
  if (!response.ok) {
    throw new Error(result.message);
  }
  return result;
};

export const login = async (data: Record<string, any>) => {
  const response = await fetch(
    `${(window as any).__APP_CONFIG__.VITE_API_URL}:${
      (window as any).__APP_CONFIG__.VITE_API_PORT
    }/api/logIn`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(data),
    }
  );
  const result = await response.json();
  if (!response.ok) {
    throw new Error(result.message);
  }
  return result;
};

export const fetchVideoList = async () => {
  const response = await fetch(
    `${(window as any).__APP_CONFIG__.VITE_API_URL}:${
      (window as any).__APP_CONFIG__.VITE_API_PORT
    }/api/list?type=video`,
    {
      headers: {
        Authorization: `Bearer ${localStorage.getItem("token")}`,
      },
    }
  );
  const result = await response.json();
  if (!response.ok) {
    if (response.status === 401) {
      localStorage.removeItem("token");
      router.replace("/login");
    }
    throw new Error(result.message);
  }
  result.data = result.data.map((e: any) => {
    return {
      id: generateUUID(),
      url: e,
    };
  });
  return result;
};

export const fetchImageList = async () => {
  const response = await fetch(
    `${(window as any).__APP_CONFIG__.VITE_API_URL}:${
      (window as any).__APP_CONFIG__.VITE_API_PORT
    }/api/list?type=image`,
    {
      headers: {
        Authorization: `Bearer ${localStorage.getItem("token")}`,
      },
    }
  );
  const result = await response.json();
  if (!response.ok) {
    if (response.status === 401) {
      localStorage.removeItem("token");
      router.replace("/login");
    }
    throw new Error(result.message);
  }
  result.data = result.data.map((e: any) => {
    return {
      id: generateUUID(),
      url: e,
    };
  });
  return result;
};
