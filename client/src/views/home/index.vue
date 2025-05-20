<script setup>
// 导入子组件
import TabBar from "@/components/TabBar.vue";
import SwiperContent from "@/components/SwiperContainer.vue";
import Images from "./components/Images.vue";
import Videos from "./components/Videos.vue";

const muted = ref(true);
provide("muted", muted);

// 定义幻灯片数据
const slides = shallowRef([
  { label: "图片", component: Images },
  { label: "视频", component: Videos },
  { label: "推荐", component: Videos },
]);

const swiperInstance = ref(null);
const activeTabIndex = ref(0);
const underlineStyle = reactive({
  left: "0px",
  width: "0px",
  transition: "left 0.3s ease-out, width 0.3s ease-out",
});
const tabsComponentRef = ref(null);

const onSwiperInitialized = (swiper) => {
  swiperInstance.value = swiper;
  nextTick(() => {
    updateUnderline(false);
  });
};

const onSlideChangeHandler = (swiper) => {
  activeTabIndex.value = swiper.activeIndex;
  underlineStyle.transition = "left 0.3s ease-out, width 0.3s ease-out";
  updateUnderline(true);
};

const onSwiperProgress = (swiper, progress) => {
  if (
    !tabsComponentRef.value ||
    !tabsComponentRef.value.getTabItemRefs ||
    !tabsComponentRef.value.getTabItemRefs().length === 0
  ) {
    return;
  }

  const tabItemRefs = tabsComponentRef.value.getTabItemRefs();
  underlineStyle.transition = "left 0.05s linear, width 0.05s linear";

  const totalTabs = slides.value.length;
  if (totalTabs <= 1) {
    updateUnderline(false);
    return;
  }

  const fractionalIndex = progress * (totalTabs - 1);
  const currentTabIndex = Math.floor(fractionalIndex);
  const nextTabIndex = Math.ceil(fractionalIndex);
  const interSlideProgress = fractionalIndex - currentTabIndex;

  const currentTabEl = tabItemRefs[currentTabIndex];
  const nextTabEl = tabItemRefs[nextTabIndex];

  if (!currentTabEl) return;

  let currentTabLeft = currentTabEl.offsetLeft;
  let currentTabWidth = currentTabEl.offsetWidth;

  if (currentTabIndex < nextTabIndex && nextTabEl) {
    const nextTabLeft = nextTabEl.offsetLeft;
    const nextTabWidth = nextTabEl.offsetWidth;

    currentTabLeft =
      currentTabLeft + (nextTabLeft - currentTabLeft) * interSlideProgress;
    currentTabWidth =
      currentTabWidth + (nextTabWidth - currentTabWidth) * interSlideProgress;
  }

  underlineStyle.left = `${currentTabLeft}px`;
  underlineStyle.width = `${currentTabWidth}px`;
};

const goToSlide = (index) => {
  if (swiperInstance.value) {
    activeTabIndex.value = index;
    underlineStyle.transition = "left 0.3s ease-out, width 0.3s ease-out";
    updateUnderline(true);
    swiperInstance.value.slideTo(index);
  }
};

const updateUnderline = (smooth = true) => {
  if (
    !tabsComponentRef.value ||
    !tabsComponentRef.value.getTabItemRefs ||
    !tabsComponentRef.value.getTabItemRefs().length === 0
  ) {
    return;
  }

  const tabItemRefs = tabsComponentRef.value.getTabItemRefs();
  if (tabItemRefs && tabItemRefs.length > activeTabIndex.value) {
    const activeTabEl = tabItemRefs[activeTabIndex.value];
    if (activeTabEl) {
      if (!smooth) {
        underlineStyle.transition = "none";
      }
      underlineStyle.left = `${activeTabEl.offsetLeft}px`;
      underlineStyle.width = `${activeTabEl.offsetWidth}px`;

      if (!smooth) {
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            underlineStyle.transition =
              "left 0.3s ease-out, width 0.3s ease-out";
          });
        });
      }
    }
  }
};

const standalone = computed(
  () => window.matchMedia("(display-mode: standalone)").matches
);
provide("standalone", standalone);

// 设置真实视口高度（解决 iOS Safari 100vh 问题）
const setRealVh = () => {
  const vh = window.innerHeight * 0.01;
  document.documentElement.style.setProperty("--vh", `${vh}px`);
};

onMounted(() => {
  setRealVh();
  window.addEventListener("resize", setRealVh);
});
</script>

<template>
  <div class="fullscreen-swiper-app" :class="{ standalone }">
    <TabBar
      :tabs="slides"
      :activeTabIndex="activeTabIndex"
      :underlineStyle="underlineStyle"
      :standalone="standalone"
      @tab-click="goToSlide"
      ref="tabsComponentRef"
    />
    <SwiperContent
      :slides="slides"
      @swiper-initialized="onSwiperInitialized"
      @slide-change="onSlideChangeHandler"
      @swiper-progress="onSwiperProgress"
    />
  </div>
</template>

<style scoped lang="less">
:root {
  --vh: 100vh;
}

.fullscreen-swiper-app {
  display: flex;
  flex-direction: column;
  height: calc(var(--vh, 1vh) * 100); /* 使用真实视口高度 */
  width: 100vw;
  overflow-x: hidden;
  overflow-y: hidden;
}
& .standalone {
  height: 100vh;
}
</style>
