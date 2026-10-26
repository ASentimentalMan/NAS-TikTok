import { flushPromises, mount } from "@vue/test-utils";
import type Swiper from "swiper";
import { nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

const { fetchVideoListMock } = vi.hoisted(() => ({ fetchVideoListMock: vi.fn() }));

vi.mock("@/apis/index", () => ({ fetchVideoList: fetchVideoListMock }));

vi.mock("@/components/VideoPlayer.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      props: {
        video: { type: Object, required: true },
        index: { type: Number, required: true },
        isActive: Boolean,
        preloadMode: String,
        feedActive: Boolean,
      },
      setup(props) {
        return () => h("video", {
          "data-video-url": props.video.url,
          "data-logical-index": props.index,
          "data-active": String(props.isActive),
          "data-preload": props.preloadMode,
          "data-feed-active": String(props.feedActive),
        });
      },
    }),
  };
});

import Videos from "@/views/home/components/Videos.vue";

const tickFrame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

describe("video pool with real Swiper", () => {
  const originalWidth = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth");
  const originalHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientHeight");

  afterEach(() => {
    fetchVideoListMock.mockReset();
    if (originalWidth) Object.defineProperty(HTMLElement.prototype, "clientWidth", originalWidth);
    else Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
    if (originalHeight) Object.defineProperty(HTMLElement.prototype, "clientHeight", originalHeight);
    else Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
  });

  it("recycles five real video nodes through eight forward and backward Swiper transitions", async () => {
    Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get: () => 390 });
    Object.defineProperty(HTMLElement.prototype, "clientHeight", { configurable: true, get: () => 844 });
    fetchVideoListMock
      .mockResolvedValueOnce({ data: Array.from({ length: 20 }, (_, index) => ({ url: `/video-${index}.mp4` })) })
      .mockResolvedValue({ data: [] });

    const wrapper = mount(Videos, {
      attachTo: document.body,
      props: { currentIndex: 1 },
      global: { provide: { activeTabIndex: ref(1) } },
    });

    try {
      await flushPromises();
      await nextTick();
      await tickFrame();
      const rootElement = wrapper.element as HTMLElement;
      const container = wrapper.find(".swiper-container").element as HTMLElement & { swiper?: Swiper };
      const swiper = container.swiper;
      expect(swiper).toBeTruthy();
      const nodes = Array.from(rootElement.querySelectorAll<HTMLVideoElement>("video"));
      expect(nodes).toHaveLength(5);
      const assertSameFiveNodes = () => {
        const currentNodes = Array.from(rootElement.querySelectorAll<HTMLVideoElement>("video"));
        expect(currentNodes).toHaveLength(5);
        expect(new Set(currentNodes).size).toBe(5);
        expect(currentNodes.every((node) => nodes.includes(node))).toBe(true);
        expect(nodes.every((node) => node.isConnected && rootElement.contains(node))).toBe(true);
      };

      const settle = async (direction: "next" | "prev", logicalIndex: number) => {
        if (direction === "next") swiper!.slideNext(0);
        else swiper!.slidePrev(0);
        await tickFrame();
        await new Promise((resolve) => setTimeout(resolve, 25));
        await flushPromises();
        await nextTick();
        expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe(String(logicalIndex));
        assertSameFiveNodes();
      };

      swiper!.swipeDirection = "next";
      swiper!.slideTo(swiper!.activeIndex + 2, 100);
      swiper!.swipeDirection = "prev";
      swiper!.slideTo(swiper!.activeIndex - 1, 100);
      expect(swiper!.realIndex).toBe(1);
      swiper!.wrapperEl.dispatchEvent(new Event("transitionend", { bubbles: true }));
      await flushPromises();
      await nextTick();
      expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("1");
      expect(nodes.find((node) => node.getAttribute("data-logical-index") === "1")?.getAttribute("data-video-url"))
        .toBe("/video-1.mp4");
      assertSameFiveNodes();
      swiper!.slideTo(swiper!.activeIndex - 1, 0);
      await tickFrame();
      await flushPromises();
      await nextTick();
      expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("0");

      swiper!.slideTo(swiper!.activeIndex + 1, 100);
      swiper!.slideTo(swiper!.activeIndex + 1, 100);
      expect(swiper!.realIndex).toBe(2);
      swiper!.wrapperEl.dispatchEvent(new Event("transitionend", { bubbles: true }));
      await flushPromises();
      await nextTick();
      expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("2");
      expect(nodes.filter((node) => node.getAttribute("data-active") === "true")).toHaveLength(1);
      expect(nodes.find((node) => node.getAttribute("data-logical-index") === "2")?.getAttribute("data-video-url"))
        .toBe("/video-2.mp4");
      assertSameFiveNodes();

      swiper!.slideTo(swiper!.activeIndex + 1, 100);
      swiper!.slideTo(swiper!.activeIndex - 1, 100);
      expect(swiper!.realIndex).toBe(2);
      swiper!.wrapperEl.dispatchEvent(new Event("transitionend", { bubbles: true }));
      await flushPromises();
      await nextTick();
      expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("2");
      expect(nodes.filter((node) => node.getAttribute("data-active") === "true")).toHaveLength(1);
      assertSameFiveNodes();

      swiper!.slideNext(0);
      await tickFrame();
      await flushPromises();
      await nextTick();
      expect(wrapper.find(".tiktok-feed").attributes("data-logical-index")).toBe("3");

      for (let logicalIndex = 4; logicalIndex <= 8; logicalIndex += 1) {
        await settle("next", logicalIndex);
        const logicalNode = nodes.find((node) => node.getAttribute("data-logical-index") === String(logicalIndex));
        expect(logicalNode?.getAttribute("data-video-url")).toBe(`/video-${logicalIndex}.mp4`);
        expect(rootElement.contains(logicalNode!)).toBe(true);
      }

      assertSameFiveNodes();
      for (let logicalIndex = 7; logicalIndex >= 0; logicalIndex -= 1) {
        await settle("prev", logicalIndex);
        if (logicalIndex <= 4) {
          const logicalNode = nodes.find((node) => node.getAttribute("data-logical-index") === String(logicalIndex));
          expect(logicalNode?.getAttribute("data-video-url")).toBe(`/video-${logicalIndex}.mp4`);
        }
      }
      assertSameFiveNodes();
    } finally {
      wrapper.unmount();
    }
  });
});
