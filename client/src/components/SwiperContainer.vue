<script setup>
// 导入 Swiper 核心和 Vue 组件
import { Swiper, SwiperSlide } from "swiper/vue";
// 导入 Swiper 样式
import "swiper/css";
// 如果需要其他模块（如 Navigation, Pagination），需要在这里导入样式和模块
// import 'swiper/css/navigation';
// import 'swiper/css/pagination';
// import { Navigation, Pagination } from 'swiper/modules';

// 定义组件接收的 props
const props = defineProps({
  slides: {
    type: Array,
    required: true,
  },
});

const activeIndex = ref(props.slides.length - 1);
provide("activeTabIndex", activeIndex);

// 定义组件可以触发的事件
const emit = defineEmits([
  "swiper-initialized",
  "slide-change",
  "swiper-progress",
]);

// 定义 Swiper 使用的模块 (根据需要导入)
const modules = []; // 当前示例没有使用额外模块

// Swiper 初始化时调用，触发 swiper-initialized 事件，并传递 Swiper 实例
const onSwiper = (swiper) => {
  emit("swiper-initialized", swiper);
};

// Swiper 滑动改变时调用，触发 slide-change 事件，并传递 Swiper 实例
const onSlideChange = (swiper) => {
  activeIndex.value = swiper.activeIndex;
  emit("slide-change", swiper);
};

// Swiper 滑动过程中调用，触发 swiper-progress 事件，并传递 Swiper 实例和进度值
const onProgress = (swiper, progress) => {
  emit("swiper-progress", swiper, progress);
};

// 注意：Swiper 实例本身不会在这个组件内被直接操作（如 slideTo），
// 这些操作将在父组件中通过接收到的 Swiper 实例进行。
</script>

<template>
  <swiper
    :modules="modules"
    :slides-per-view="1"
    :space-between="0"
    :watch-slides-progress="true"
    :initial-slide="props.slides.length - 1"
    @swiper="onSwiper"
    @slideChange="onSlideChange"
    @progress="onProgress"
    class="main-swiper"
  >
    <swiper-slide v-for="(slide, index) in slides" :key="index">
      <component :is="slide.component" :currentIndex="index" />
    </swiper-slide>
  </swiper>
</template>

<style scoped>
/* Swiper 主容器样式 */
.main-swiper {
  width: 100%;
  height: 100%; /* 占满剩余的垂直空间 */
  flex-grow: 1; /* 允许 Swiper 填充可用空间 */
}

/* 单个幻灯片样式 */
.swiper-slide {
  display: flex;
  justify-content: center; /* 内容水平居中 */
  align-items: center; /* 内容垂直居中 */
  width: 100%;
  height: 100%;
  box-sizing: border-box;
}

/* 幻灯片内容容器样式 */
.slide-content-wrapper {
  padding: 30px;
  text-align: center;
  max-width: 80%;
}

.slide-content-wrapper h1 {
  font-size: 2.5rem;
  color: #333;
  margin-bottom: 1rem;
}

.slide-content-wrapper p {
  font-size: 1.1rem;
  color: #666;
  line-height: 1.6;
}
</style>
