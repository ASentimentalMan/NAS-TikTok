<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { Swiper, SwiperSlide } from "swiper/vue";
import "swiper/css";

import VideoPlayer from "@/components/VideoPlayer.vue";
import { fetchVideoList } from "@/apis/index";

const props = defineProps({
  currentIndex: {
    type: Number,
    default: 0,
  },
});

const activeTabIndex = inject("activeTabIndex", ref(1));

const SLOT_COUNT = 5;
const EMPTY_VIDEO = Object.freeze({ id: "empty", url: "" });
const slotIds = Array.from({ length: SLOT_COUNT }, (_, slotId) => slotId);
const slots = ref(
  slotIds.map((slotId) => ({
    slotId,
    logicalIndex: -1,
    video: EMPTY_VIDEO,
  }))
);
const logicalHistory = ref([]);
const currentLogicalIndex = ref(0);
const navigationLogicalIndex = ref(0);
const swiperInstance = ref(null);
let nextOccurrenceId = 0;
let isFetching = false;
let activeRequest = null;
let retryTimer = null;
let retryAt = 0;
let consecutiveFailures = 0;
let isUnmounted = false;
let lastSettledSlot = 0;
let initialized = false;
const isTransitioning = ref(false);
let pendingTransition = null;
let exhausted = false;

const clearRetry = () => {
  if (retryTimer !== null) clearTimeout(retryTimer);
  retryTimer = null;
};

const scheduleRetry = () => {
  consecutiveFailures += 1;
  const delay = Math.min(1000 * 2 ** (consecutiveFailures - 1), 30000);
  retryAt = Date.now() + delay;
  clearRetry();
  retryTimer = setTimeout(() => {
    retryTimer = null;
    if (!isUnmounted) void fetchData();
  }, delay);
};

const ensureOccurrence = (logicalIndex) => {
  if (logicalIndex < 0) return null;
  return logicalHistory.value[logicalIndex] || null;
};

const syncSlots = () => {
  const current = currentLogicalIndex.value;
  const firstLogicalIndex = Math.max(0, current - 2);
  const lastLogicalIndex = current + 2;
  const slotContents = new Map();

  for (
    let logicalIndex = firstLogicalIndex;
    logicalIndex <= lastLogicalIndex;
    logicalIndex += 1
  ) {
    const video = ensureOccurrence(logicalIndex);
    if (video) slotContents.set(logicalIndex % SLOT_COUNT, { logicalIndex, video });
  }

  slots.value = slots.value.map((slot) => {
    const content = slotContents.get(slot.slotId);
    if (!content) {
      if (slot.video === EMPTY_VIDEO && slot.logicalIndex === -1) return slot;
      return { ...slot, logicalIndex: -1, video: EMPTY_VIDEO };
    }
    if (slot.logicalIndex === content.logicalIndex && slot.video === content.video) return slot;
    return { ...slot, logicalIndex: content.logicalIndex, video: content.video };
  });
};

const initializeHistory = () => {
  if (initialized || logicalHistory.value.length === 0) return;
  initialized = true;
  currentLogicalIndex.value = 0;
  navigationLogicalIndex.value = 0;
  lastSettledSlot = swiperInstance.value?.realIndex % SLOT_COUNT || 0;
  syncSlots();
  if (swiperInstance.value && !swiperInstance.value.destroyed) {
    swiperInstance.value.allowSlidePrev = false;
    swiperInstance.value.allowSlideNext = logicalHistory.value.length > 1;
    swiperInstance.value.slideToLoop(0, 0, false);
  }
};

const updateSlidePermissions = () => {
  const swiper = swiperInstance.value;
  if (!swiper || swiper.destroyed) return;
  swiper.allowSlidePrev = initialized && navigationLogicalIndex.value > 0;
  swiper.allowSlideNext =
    initialized && navigationLogicalIndex.value + 1 < logicalHistory.value.length;
};

const maybeFetchMore = () => {
  if (
    initialized &&
    !exhausted &&
    currentLogicalIndex.value >= logicalHistory.value.length - 5
  ) {
    void fetchData();
  }
};

const fetchData = async () => {
  if (isUnmounted || isFetching || Date.now() < retryAt) return;

  const controller = new AbortController();
  activeRequest = controller;
  isFetching = true;
  try {
    const response = await fetchVideoList(controller.signal);
    if (controller.signal.aborted || isUnmounted) return;
    const list = Array.isArray(response.data) ? response.data : [];
    if (list.length > 0) {
      consecutiveFailures = 0;
      clearRetry();
      retryAt = 0;
      logicalHistory.value.push(
        ...list.map((item) => ({
          id: `${props.currentIndex}-occurrence-${nextOccurrenceId++}`,
          url: typeof item === "string" ? item : item?.url,
        }))
      );
      initializeHistory();
      if (!isTransitioning.value) syncSlots();
      updateSlidePermissions();
    } else {
      exhausted = true;
      clearRetry();
      retryAt = Number.POSITIVE_INFINITY;
      updateSlidePermissions();
    }
  } catch (error) {
    if (!controller.signal.aborted && !isUnmounted) {
      if (error?.retryable === true) {
        scheduleRetry();
      } else {
        clearRetry();
        retryAt = Number.POSITIVE_INFINITY;
        exhausted = true;
        updateSlidePermissions();
      }
      console.error("Error loading more videos:", error);
    }
  } finally {
    if (activeRequest === controller) activeRequest = null;
    isFetching = false;
    if (logicalHistory.value.length === 1 && !exhausted) void fetchData();
  }
};

const onSwiper = (swiper) => {
  swiperInstance.value = swiper;
  lastSettledSlot = swiper.realIndex % SLOT_COUNT;
  navigationLogicalIndex.value = currentLogicalIndex.value;
  updateSlidePermissions();
  initializeHistory();
};

const onTransitionStart = (swiper) => {
  if (!initialized || swiper.destroyed) return;
  const targetSlot = swiper.realIndex % SLOT_COUNT;
  const forwardDistance = (targetSlot - lastSettledSlot + SLOT_COUNT) % SLOT_COUNT;
  const backwardDistance = (lastSettledSlot - targetSlot + SLOT_COUNT) % SLOT_COUNT;
  const distance = forwardDistance <= 2 ? forwardDistance : -backwardDistance;
  const targetLogicalIndex = currentLogicalIndex.value + distance;
  isTransitioning.value = true;
  navigationLogicalIndex.value =
    targetLogicalIndex >= 0 && targetLogicalIndex < logicalHistory.value.length
      ? targetLogicalIndex
      : currentLogicalIndex.value;
  pendingTransition = {
    fromSlot: lastSettledSlot,
    toSlot: targetSlot,
    targetLogicalIndex:
      targetLogicalIndex >= 0 && targetLogicalIndex < logicalHistory.value.length
        ? targetLogicalIndex
        : null,
  };
  updateSlidePermissions();
};

const onTransitionEnd = (swiper) => {
  if (!initialized || swiper.destroyed || !pendingTransition) return;
  const settledSlot = swiper.realIndex % SLOT_COUNT;
  const transition = pendingTransition;
  pendingTransition = null;
  isTransitioning.value = false;
  if (settledSlot !== transition.toSlot) {
    navigationLogicalIndex.value = currentLogicalIndex.value;
    swiper.slideToLoop(lastSettledSlot, 0, false);
    updateSlidePermissions();
    return;
  }
  if (settledSlot === transition.fromSlot) {
    navigationLogicalIndex.value = currentLogicalIndex.value;
    updateSlidePermissions();
    return;
  }

  const forwardDistance = (settledSlot - transition.fromSlot + SLOT_COUNT) % SLOT_COUNT;
  const backwardDistance = (transition.fromSlot - settledSlot + SLOT_COUNT) % SLOT_COUNT;
  const direction = forwardDistance <= 2 ? "forward" : "backward";
  const distance = direction === "forward" ? forwardDistance : backwardDistance;
  if (
    distance < 1 ||
    distance > 2 ||
    transition.targetLogicalIndex === null
  ) {
    navigationLogicalIndex.value = currentLogicalIndex.value;
    swiper.slideToLoop(lastSettledSlot, 0, false);
    updateSlidePermissions();
    return;
  }

  currentLogicalIndex.value = transition.targetLogicalIndex;
  navigationLogicalIndex.value = transition.targetLogicalIndex;
  lastSettledSlot = settledSlot;
  syncSlots();
  updateSlidePermissions();
  maybeFetchMore();
};

const preloadModeFor = (logicalIndex) => {
  if (
    activeTabIndex.value !== props.currentIndex ||
    logicalIndex < 0 ||
    isTransitioning.value
  ) {
    return "none";
  }
  const offset = logicalIndex - currentLogicalIndex.value;
  if (offset === 0 || offset === 1) return "auto";
  if (offset === 2 || offset === -1) return "metadata";
  return "none";
};

onMounted(() => {
  void fetchData();
});

onUnmounted(() => {
  isUnmounted = true;
  clearRetry();
  activeRequest?.abort();
});
</script>

<template>
  <div
    class="tiktok-feed"
    :data-logical-index="initialized ? currentLogicalIndex : ''"
    :data-history-length="logicalHistory.length"
  >
    <swiper
      direction="vertical"
      :slides-per-view="1"
      :space-between="0"
      :loop="true"
      :allow-slide-prev="initialized && navigationLogicalIndex > 0"
      :allow-slide-next="initialized && navigationLogicalIndex + 1 < logicalHistory.length"
      @swiper="onSwiper"
      @slideChangeTransitionStart="onTransitionStart"
      @transitionEnd="onTransitionEnd"
      class="swiper-container"
      no-swiping-class="swiper-no-swiping"
    >
      <swiper-slide
        v-for="slot in slots"
        :key="slot.slotId"
        class="swiper-slide-item"
      >
        <VideoPlayer
          :video="slot.video"
          :index="slot.logicalIndex"
          :isActive="
            !isTransitioning &&
            activeTabIndex == props.currentIndex &&
            slot.logicalIndex === currentLogicalIndex
          "
          :randomPlay="props.currentIndex == 2"
          :preloadMode="preloadModeFor(slot.logicalIndex)"
          :feedActive="activeTabIndex == props.currentIndex"
        />
      </swiper-slide>
    </swiper>
  </div>
</template>

<style scoped>
.tiktok-feed {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
  background-color: #000;
}

.swiper-container {
  width: 100%;
  height: 100%;
}

.swiper-slide-item {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
  height: 100%;
  background-color: #111;
}
</style>
