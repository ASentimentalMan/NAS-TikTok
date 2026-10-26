import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
};

describe("VideoPlayer playback lifecycle", () => {
  beforeEach(() => {
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  });
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
      props: { video: { id: "one", url: "/video.mp4" }, index: 0, isActive: false },
    });

    await wrapper.setProps({ isActive: true });
    Object.defineProperty(wrapper.find("video").element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(wrapper.find("video").element, "duration", { configurable: true, value: 30 });
    wrapper.find("video").element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(playMock).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    pendingPlay.reject(new DOMException("interrupted", "AbortError"));
    await flushPromises();

    expect(playMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the video node when its occurrence is recycled and ignores stale play resolution", async () => {
    const firstPlay = deferred<void>();
    const playMock = vi.spyOn(HTMLMediaElement.prototype, "play")
      .mockReturnValueOnce(firstPlay.promise)
      .mockResolvedValue(undefined);
    const pauseMock = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    const loadMock = vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    const wrapper = mount(VideoPlayer, {
      props: { video: { id: "one", url: "/one.mp4", currentTime: 0 }, index: 0, isActive: true },
    });
    const element = wrapper.find("video").element;

    Object.defineProperty(element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(element, "duration", { configurable: true, value: 30 });
    element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(playMock).toHaveBeenCalledTimes(1);
    await wrapper.setProps({ video: { id: "two", url: "/two.mp4", currentTime: 0 }, index: 5, isActive: true });
    expect(wrapper.find("video").element).toBe(element);
    expect(loadMock).toHaveBeenCalled();
    pauseMock.mockClear();
    Object.defineProperty(element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(element, "duration", { configurable: true, value: 40 });
    element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(playMock).toHaveBeenCalledTimes(2);
    firstPlay.resolve();
    await flushPromises();

    expect(wrapper.find("video").element).toBe(element);
    expect(pauseMock).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("does not load a source again on feed activation and uses preload policy for inactive tabs", async () => {
    const loadMock = vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    const wrapper = mount(VideoPlayer, {
      props: {
        video: { id: "one", url: "/one.mp4" }, index: 0, isActive: false,
        feedActive: false, preloadMode: "none",
      },
    });
    const element = wrapper.find("video").element;
    expect(loadMock).toHaveBeenCalledTimes(0);
    await wrapper.setProps({ feedActive: true, preloadMode: "metadata" });
    expect(wrapper.find("video").element).toBe(element);
    expect(loadMock).toHaveBeenCalledTimes(1);
    expect(element.preload).toBe("metadata");
    await wrapper.setProps({ feedActive: false, preloadMode: "none" });
    await wrapper.setProps({ feedActive: true, preloadMode: "metadata" });
    expect(loadMock).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("prepares the recommended target while inactive and preserves it on activation", async () => {
    const random = vi.spyOn(Math, "random").mockReturnValue(0.5);
    const video = { id: "recommendation", url: "/recommended.mp4", randomStartTime: 12 };
    const playMock = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    const wrapper = mount(VideoPlayer, {
      props: { video, index: 2, isActive: false, randomPlay: true, preloadMode: "auto", feedActive: true },
    });
    const element = wrapper.find("video").element;
    Object.defineProperty(element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(element, "duration", { configurable: true, value: 30 });
    Object.defineProperty(element, "buffered", { configurable: true, value: {
      length: 1, start: () => 10, end: () => 16,
    } });
    element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(element.currentTime).toBe(12);
    expect(element.getAttribute("data-buffered-seconds")).toBe("4");
    expect(random).not.toHaveBeenCalled();
    await wrapper.setProps({ isActive: true });
    expect(element.currentTime).toBe(12);
    expect(random).not.toHaveBeenCalled();
    expect(playMock).toHaveBeenCalled();
    wrapper.unmount();
  });

  it.each([
    ["short of three seconds", 0, 2.9, 20, "2.9"],
    ["three seconds", 0, 3, 20, "3"],
    ["buffered elsewhere", 8, 12, 20, "0"],
    ["remaining short clip", 0, 2, 2, "2"],
  ])("measures a target-containing prebuffer when %s", async (_label, start, end, duration, expected) => {
    const video = { id: `buffer-${_label}`, url: "/buffer.mp4", currentTime: 0 };
    const wrapper = mount(VideoPlayer, {
      props: { video, index: 0, isActive: false, preloadMode: "auto", feedActive: true },
    });
    const element = wrapper.find("video").element;
    Object.defineProperty(element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(element, "duration", { configurable: true, value: duration });
    Object.defineProperty(element, "buffered", { configurable: true, value: {
      length: 1, start: () => start, end: () => end,
    } });
    element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(element.getAttribute("data-buffered-seconds")).toBe(expected);
    wrapper.unmount();
  });

  it("chooses a random recommendation target once and keeps it through activation", async () => {
    const random = vi.spyOn(Math, "random").mockReturnValue(0.25);
    const video: { id: string; url: string; randomStartTime?: number; currentTime?: number } = {
      id: "random-recommendation", url: "/random.mp4",
    };
    const wrapper = mount(VideoPlayer, {
      props: { video, index: 2, isActive: false, randomPlay: true, preloadMode: "auto", feedActive: true },
    });
    const element = wrapper.find("video").element;
    Object.defineProperty(element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(element, "duration", { configurable: true, value: 20 });
    element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    const chosenTarget = video.randomStartTime;
    expect(chosenTarget).toBe(3.75);
    expect(element.currentTime).toBe(chosenTarget);
    expect(random).toHaveBeenCalledTimes(1);
    await wrapper.setProps({ isActive: true });
    expect(element.currentTime).toBe(chosenTarget);
    expect(random).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("uses a bounded HEAD confirmation and exits only on an explicit 401", async () => {
    const pauseMock = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    const fetchMock = vi.fn().mockResolvedValue({ status: 401, ok: false });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(VideoPlayer, {
      props: { video: { id: "auth", url: "/api/stream/video.mp4" }, index: 0, isActive: true },
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
      props: { video: { id: "timeout", url: "/api/stream/video.mp4" }, index: 0, isActive: true },
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

  it("ignores a stale HEAD 401 after the same node is rebound", async () => {
    const head = deferred<Response>();
    vi.stubGlobal("fetch", vi.fn().mockReturnValue(head.promise));
    const wrapper = mount(VideoPlayer, {
      props: { video: { id: "old", url: "/old.mp4" }, index: 0, isActive: true },
    });
    const element = wrapper.find("video").element;
    element.dispatchEvent(new Event("error"));
    await flushPromises();
    await wrapper.setProps({ video: { id: "new", url: "/new.mp4" }, index: 5 });
    head.resolve({ status: 401, ok: false } as Response);
    await flushPromises();

    expect(wrapper.find("video").element).toBe(element);
    expect(routerMock.replace).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("starts a fresh HEAD check after the same occurrence is deactivated and reactivated", async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(VideoPlayer, {
      props: { video: { id: "same", url: "/same.mp4" }, index: 0, isActive: true },
    });
    const element = wrapper.find("video").element;
    element.dispatchEvent(new Event("error"));
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await wrapper.setProps({ isActive: false });
    await wrapper.setProps({ isActive: true });
    element.dispatchEvent(new Event("error"));
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it.each([403, 404, 500, 503])("does not treat HEAD %i as an expired session", async (status) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status, ok: status < 400 }));
    const wrapper = mount(VideoPlayer, {
      props: { video: { id: `status-${status}`, url: "/api/stream/video.mp4" }, index: 0, isActive: true },
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
      props: { video: { id: "active", url: "/api/stream/video.mp4" }, index: 0, isActive: false },
    });

    await wrapper.setProps({ isActive: true });
    Object.defineProperty(wrapper.find("video").element, "readyState", { configurable: true, value: 1 });
    Object.defineProperty(wrapper.find("video").element, "duration", { configurable: true, value: 30 });
    wrapper.find("video").element.dispatchEvent(new Event("loadedmetadata"));
    await flushPromises();
    expect(playMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).not.toHaveBeenCalled();
    await wrapper.setProps({ isActive: false });
    expect(fetchMock).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
