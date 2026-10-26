import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
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
    emits: ["swiper", "slideChange", "slideChangeTransitionStart", "transitionEnd", "autoplayStop"],
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
    default: component({
      name: "MockImagePlayer",
      props: { image: { type: Object, required: true } },
      setup(props) {
        return () => createElement("div", {
          class: "mock-media-item",
          "data-media-id": props.image.id,
          "data-media-url": props.image.url,
        });
      },
    }),
  };
});

vi.mock("@/components/VideoPlayer.vue", async () => {
  const { defineComponent: component, h: createElement } = await import("vue");
  return {
    default: component({
      name: "MockVideoPlayer",
      props: {
        video: { type: Object, required: true }, index: Number, isActive: Boolean,
        randomPlay: Boolean, preloadMode: String, feedActive: Boolean,
      },
      setup(props) {
        return () => createElement("video", {
          class: "mock-media-item",
          "data-media-id": props.video.id,
          "data-media-url": props.video.url,
          "data-index": props.index,
          "data-active": String(props.isActive),
          "data-feed-active": String(props.feedActive),
          "data-preload": props.preloadMode,
          preload: props.preloadMode,
          muted: !props.feedActive || !props.isActive,
        });
      },
    }),
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
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("preserves image URLs returned more than once across API batches", async () => {
    fetchImageListMock
      .mockResolvedValueOnce(items(["/one.jpg", "/two.jpg"]))
      .mockResolvedValueOnce(items(["/two.jpg", "/three.jpg", "/three.jpg"]));
    const wrapper = mount(Images);
    await flushPromises();

    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(2);
    await wrapper.findComponent({ name: "MockSwiper" }).vm.$emit("slideChange", { activeIndex: 0 });
    await flushPromises();

    expect(fetchImageListMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(5);
    expect(wrapper.findAll(".mock-media-item").map((item) => item.attributes("data-media-url")))
      .toEqual(["/one.jpg", "/two.jpg", "/two.jpg", "/three.jpg", "/three.jpg"]);
    const ids = wrapper.findAll(".mock-media-item").map((item) => item.attributes("data-media-id"));
    expect(new Set(ids).size).toBe(5);
    wrapper.unmount();
  });

  it("keeps five video slots while appending repeated URLs as distinct occurrences", async () => {
    fetchVideoListMock.mockResolvedValueOnce(items(["/one.mp4", "/two.mp4", "/two.mp4", "/three.mp4", "/three.mp4", "/six.mp4", "/seven.mp4", "/eight.mp4"])).mockResolvedValue(items([]));
    const wrapper = mount(Videos);
    await flushPromises();

    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(5);
    const slotElements = wrapper.findAll(".swiper-slide-item").map((item) => item.element);
    expect(wrapper.findAll(".mock-media-item").map((item) => item.attributes("data-media-url")))
      .toEqual(["/one.mp4", "/two.mp4", "/two.mp4", "", ""]);
    const swiper = wrapper.findComponent({ name: "MockSwiper" });
    for (let logicalIndex = 1; logicalIndex <= 7; logicalIndex += 1) {
      const state = { realIndex: logicalIndex % 5, swipeDirection: "next", destroyed: false };
      await swiper.vm.$emit("slideChangeTransitionStart", state);
      await swiper.vm.$emit("transitionEnd", state);
    }
    expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("7");
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(5);
    expect(wrapper.findAll(".swiper-slide-item").map((item) => item.element)).toEqual(slotElements);
    expect(wrapper.findAll(".mock-media-item").map((item) => item.attributes("data-media-url")))
      .toEqual(["/six.mp4", "/seven.mp4", "/eight.mp4", "", ""]);
    const visibleOccurrences = wrapper.findAll(".mock-media-item").filter((item) => item.attributes("data-media-url"));
    expect(new Set(visibleOccurrences.map((item) => item.attributes("data-media-id"))).size).toBe(3);
    for (let logicalIndex = 6; logicalIndex >= 0; logicalIndex -= 1) {
      const state = { realIndex: logicalIndex % 5, swipeDirection: "prev", destroyed: false };
      await swiper.vm.$emit("slideChangeTransitionStart", state);
      await swiper.vm.$emit("transitionEnd", state);
    }
    expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("0");
    wrapper.unmount();
  });

  it("keeps tab histories independent and gates active playback and preload by tab", async () => {
    fetchVideoListMock
      .mockResolvedValueOnce(items(["/tab-zero.mp4", "/zero-next.mp4"]))
      .mockResolvedValueOnce(items(["/tab-one.mp4", "/one-next.mp4"]));
    const activeTabIndex = ref(0);
    const wrapper = mount({
      components: { Videos },
      template: "<div><Videos :current-index='0'/><Videos :current-index='1'/></div>",
    }, { global: { provide: { activeTabIndex } } });
    await flushPromises();

    const feeds = wrapper.findAll(".tiktok-feed");
    expect(feeds).toHaveLength(2);
    expect(feeds[0].attributes("data-logical-index")).toBe("0");
    expect(feeds[1].attributes("data-logical-index")).toBe("0");
    expect(feeds[0].findAll("video")[0].attributes("data-active")).toBe("true");
    expect(feeds[1].findAll("video").every((node) => node.attributes("data-feed-active") === "false")).toBe(true);
    expect(feeds[1].findAll("video").every((node) => node.attributes("data-preload") === "none")).toBe(true);
    activeTabIndex.value = 1;
    await flushPromises();
    expect(feeds[0].findAll("video").every((node) => node.attributes("data-active") === "false")).toBe(true);
    expect(feeds[1].findAll("video")[0].attributes("data-active")).toBe("true");
    wrapper.unmount();
  });

  it("locks concurrent image requests and aborts on unmount", async () => {
    const fetchMock = fetchImageListMock;
    const component = Images;
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
  ] as const)("recovers %s after an initial transient request failure", async (name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock
      .mockRejectedValueOnce(Object.assign(new Error("temporary failure"), { retryable: true }))
      .mockResolvedValueOnce(items(name === "videos" ? ["/recovered-one.media", "/recovered-two.media"] : ["/recovered.media"]))
      .mockResolvedValue(items([]));
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(name === "videos" ? 5 : 0);
    if (name === "videos") expect(wrapper.findAll("video").every((node) => !node.attributes("src"))).toBe(true);

    await vi.advanceTimersByTimeAsync(1000);
    await flushMicrotasks();
    await wrapper.vm.$nextTick();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(name === "videos" ? 5 : 1);
    if (name === "videos") {
      expect(wrapper.findAll("video").some((node) => node.attributes("data-media-url") === "/recovered-one.media")).toBe(true);
      expect(wrapper.findAll("video").some((node) => node.attributes("data-media-url") === "/recovered-two.media")).toBe(true);
      expect(fetchMock).toHaveBeenCalledTimes(2);
    }
    wrapper.unmount();
  });

  it.each([
    ["images", Images, fetchImageListMock],
    ["videos", Videos, fetchVideoListMock],
  ] as const)("does not retry an empty %s result", async (name, component, fetchMock) => {
    vi.useFakeTimers();
    fetchMock.mockResolvedValue(items([]));
    const wrapper = mount(component);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await wrapper.findComponent({ name: "MockSwiper" }).vm.$emit("slideChange", { activeIndex: 0 });
    await vi.advanceTimersByTimeAsync(120000);
    await flushMicrotasks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll(".swiper-slide-item")).toHaveLength(name === "videos" ? 5 : 0);
    if (name === "videos") expect(wrapper.findAll("video").every((node) => !node.attributes("data-media-url"))).toBe(true);
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
