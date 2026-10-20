import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import SimpleToast from "@/components/SimpleToast.vue";

describe("component timer cleanup", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("clears the toast timer when the component unmounts", () => {
    vi.useFakeTimers();
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    const wrapper = mount(SimpleToast);

    (wrapper.vm as any).show("notice", 1000);
    wrapper.unmount();

    expect(clearTimeoutSpy).toHaveBeenCalled();
  });
});
