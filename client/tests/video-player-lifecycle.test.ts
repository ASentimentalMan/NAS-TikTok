import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@iconify/vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { Icon: defineComponent({ setup: () => () => h("span") }) };
});

vi.mock("@/components/Loading.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@/components/BaseMusic.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@/components/SimpleToast.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      setup(_props, { expose }) {
        expose({ show: () => {} });
        return () => h("div");
      },
    }),
  };
});

const routerMock = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("@/router", () => ({ default: routerMock }));

import VideoPlayer from "@/components/VideoPlayer.vue";

const deferred = <T,>() => {
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((_resolve, rejectPromise) => {
    reject = rejectPromise;
  });
  return { promise, reject };
};

describe("VideoPlayer playback lifecycle", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    routerMock.replace.mockReset();
  });

  it("ignores a delayed play rejection after unmount", async () => {
    const pendingPlay = deferred<void>();
    const playMock = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockReturnValue(pendingPlay.promise);
    const wrapper = mount(VideoPlayer, {
      props: { video: { url: "/video.mp4" }, index: 0, isActive: false },
    });

    await wrapper.setProps({ isActive: true });
    expect(playMock).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    pendingPlay.reject(new DOMException("interrupted", "AbortError"));
    await flushPromises();

    expect(playMock).toHaveBeenCalledTimes(1);
  });

  it("uses a bounded HEAD confirmation and exits only on an explicit 401", async () => {
    const pauseMock = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    const fetchMock = vi.fn().mockResolvedValue({ status: 401, ok: false });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(VideoPlayer, {
      props: { video: { url: "/api/stream/video.mp4" }, index: 0, isActive: false },
    });

    wrapper.find("video").element.dispatchEvent(new Event("error"));
    await flushPromises();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/stream/video.mp4", expect.objectContaining({
      method: "HEAD",
      credentials: "include",
      signal: expect.any(AbortSignal),
    }));
    expect(pauseMock).toHaveBeenCalled();
    expect(routerMock.replace).toHaveBeenCalledWith("/login");
    wrapper.unmount();
  });

  it("aborts a hung HEAD after five seconds without treating it as session expiry", async () => {
    vi.useFakeTimers();
    let headSignal!: AbortSignal;
    const fetchMock = vi.fn((_url: string, options: RequestInit) => {
      headSignal = options.signal as AbortSignal;
      return new Promise<Response>((_resolve, reject) => {
        headSignal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), {
          once: true,
        });
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(VideoPlayer, {
      props: { video: { url: "/api/stream/video.mp4" }, index: 0, isActive: false },
    });

    wrapper.find("video").element.dispatchEvent(new Event("error"));
    await vi.advanceTimersByTimeAsync(4999);
    expect(headSignal.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();

    expect(headSignal.aborted).toBe(true);
    expect(routerMock.replace).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it.each([403, 404, 500, 503])("does not treat HEAD %i as an expired session", async (status) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status, ok: status < 400 }));
    const wrapper = mount(VideoPlayer, {
      props: { video: { url: "/api/stream/video.mp4" }, index: 0, isActive: false },
    });

    wrapper.find("video").element.dispatchEvent(new Event("error"));
    await flushPromises();

    expect(routerMock.replace).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps active-slide playback independent of HEAD timeout handling", async () => {
    const playMock = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(VideoPlayer, {
      props: { video: { url: "/api/stream/video.mp4" }, index: 0, isActive: false },
    });

    await wrapper.setProps({ isActive: true });
    expect(playMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).not.toHaveBeenCalled();
    await wrapper.setProps({ isActive: false });
    expect(fetchMock).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
