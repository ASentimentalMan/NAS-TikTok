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
  return { default: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@/router", () => ({ default: { replace: vi.fn() } }));

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
    vi.restoreAllMocks();
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
});
