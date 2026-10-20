import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { fetchImageListMock, fetchVideoListMock } = vi.hoisted(() => ({
  fetchImageListMock: vi.fn(),
  fetchVideoListMock: vi.fn(),
}));

vi.mock("@/apis/index", () => ({
  fetchImageList: fetchImageListMock,
  fetchVideoList: fetchVideoListMock,
}));

vi.mock("swiper/modules", () => ({ Virtual: {}, Autoplay: {} }));

vi.mock("swiper/vue", async () => {
  const { defineComponent: component, h: createElement } = await import("vue");
  const Swiper = component({
    name: "MockSwiper",
    inheritAttrs: false,
    emits: ["swiper", "slideChange", "autoplayStop"],
    setup(_props, { attrs, slots }) {
      return () => createElement("div", attrs, slots.default?.());
    },
  });
  const SwiperSlide = component({
    name: "MockSwiperSlide",
    inheritAttrs: false,
    setup(_props, { attrs, slots }) {
      return () => createElement("div", attrs, slots.default?.());
    },
  });
  return { Swiper, SwiperSlide };
});

vi.mock("@/components/ImagePlayer.vue", async () => {
  const { defineComponent: component, h: createElement } = await import("vue");
  return {
    default: component({ name: "MockImagePlayer", setup: () => () => createElement("div") }),
  };
});

vi.mock("@/components/VideoPlayer.vue", async () => {
  const { defineComponent: component, h: createElement } = await import("vue");
  return {
    default: component({ name: "MockVideoPlayer", setup: () => () => createElement("div") }),
  };
});

import Images from "@/views/home/components/Images.vue";
import Videos from "@/views/home/components/Videos.vue";

const items = (urls: string[]) => ({ data: urls.map((url) => ({ id: url, url })) });
const pendingUntilAbort = (signal?: AbortSignal) =>
  new Promise<{ data: { id: string; url: string }[] }>((_resolve, reject) => {
    if (!signal) {
      reject(new Error("request signal was not passed"));
      return;
    }
    signal.addEventListener(
      "abort",
      () => reject(new DOMException("aborted", "AbortError")),
      { once: true }
    );
  });
const flushMicrotasks = async () => {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
};

describe("media feed request lifecycle", () => {
  beforeEach(() => {
    fetchImageListMock.mockReset();
    fetchVideoListMock.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("deduplicates image URLs across consecutive API batches", async () => {
    fetchImageListMock
      .mockResolvedValueOnce(items(["/one.jpg", "/two.jpg"]))
      .mockResolvedValueOnce(items(["/two.jpg", "/three.jpg", "/three.jpg"]));
    const wrapper = mount(Images);
    await flushPromises();

    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(2);
    await wrapper.findComponent({ name: "MockSwiper" }).vm.$emit("slideChange", { activeIndex: 0 });
    await flushPromises();

    expect(fetchImageListMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(3);
    wrapper.unmount();
  });

  it("deduplicates video URLs across consecutive API batches", async () => {
    fetchVideoListMock
      .mockResolvedValueOnce(items(["/one.mp4", "/two.mp4"]))
      .mockResolvedValueOnce(items(["/two.mp4", "/three.mp4", "/three.mp4"]));
    const wrapper = mount(Videos);
    await flushPromises();

    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(2);
    await wrapper.findComponent({ name: "MockSwiper" }).vm.$emit("slideChange", { activeIndex: 0 });
    await flushPromises();

    expect(fetchVideoListMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(3);
    wrapper.unmount();
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("locks concurrent %s requests and aborts on unmount", async (_name, component, fetchMock) => {
    fetchMock.mockImplementation(pendingUntilAbort);
    const wrapper = mount(component);
    const swiper = wrapper.findComponent({ name: "MockSwiper" });

    await swiper.vm.$emit("slideChange", { activeIndex: 0 });
    await swiper.vm.$emit("slideChange", { activeIndex: 0 });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const signal = fetchMock.mock.calls[0][0];
    expect(signal?.aborted).toBe(false);
    wrapper.unmount();
    expect(signal?.aborted).toBe(true);
    await flushPromises();
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("recovers %s after an initial transient request failure", async (_name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock
      .mockRejectedValueOnce(Object.assign(new Error("temporary failure"), { retryable: true }))
      .mockResolvedValueOnce(items(["/recovered.media"]));
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(0);

    await vi.advanceTimersByTimeAsync(1000);
    await flushMicrotasks();
    await wrapper.vm.$nextTick();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(1);
    wrapper.unmount();
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("retries an empty %s result and loads later items", async (_name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock.mockResolvedValueOnce(items([])).mockResolvedValueOnce(items(["/later.media"]));
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1000);
    await flushMicrotasks();
    await wrapper.vm.$nextTick();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(1);
    wrapper.unmount();
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("cancels a pending %s retry on unmount", async (_name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock.mockRejectedValueOnce(
      Object.assign(new Error("temporary failure"), { retryable: true })
    );
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(5000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("does not retry %s after an unauthorized response", async (_name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock.mockRejectedValueOnce(
      Object.assign(new Error("unauthorized"), { status: 401, retryable: false })
    );
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(60000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});
