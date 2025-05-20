<script setup>
// 引入 Swiper Vue 组件
import { Swiper, SwiperSlide } from "swiper/vue";
// 引入 Swiper 核心样式和所需模块的样式
import "swiper/css";
import "swiper/css/virtual"; // 虚拟模块样式

// 引入 Swiper 模块
import { Virtual } from "swiper/modules";

// 引入 VideoPlayer 组件
import VideoPlayer from "@/components/VideoPlayer.vue";
import { fetchVideoList } from "@/apis/index";
const props = defineProps({
  currentIndex: {
    type: Number,
    default: 0,
  },
});
const activeTabIndex = inject("activeTabIndex", {
  type: Number,
  default: 1,
});
// 静音状态
const muted = inject("muted", {
  type: Boolean,
  default: true,
});
// 模块只保留 Virtual
const modules = [Virtual];

// 视频数据
const videos = ref([]);

const swiperInstance = ref(null);
// playingIndex 和 isPlaying 状态现在主要由 Swiper 的 activeIndex 驱动
// 如果需要全局的播放状态，可以根据 swiperInstance?.activeIndex 来判断
// 加载视频数据的方法
const fetchData = async () => {
  try {
    const response = await fetchVideoList();
    const list = response.data;
    if (list && list.length > 0) {
      videos.value = videos.value.concat(list);
      console.log("More data loaded, total videos:", videos.value.length);

      // 通知 Swiper Virtual 模块数据已更新
      if (swiperInstance.value && swiperInstance.value.virtual) {
        // virtual.update(true) is crucial after adding more data
        swiperInstance.value.virtual.update(true);
        console.log("Swiper virtual updated");
      }
    } else {
      console.log("No more data to load.");
    }
  } catch (error) {
    console.error("Error loading more videos:", error);
  }
};

// --- Swiper 事件和视频控制 ---
const onSwiper = (swiper) => {
  swiperInstance.value = swiper;
  console.log("Swiper initialized");
  // 可以在这里做一些初始化操作
  // 第一个视频的播放由 VideoPlayer 组件自身的 onMounted 和 isActive 监听器处理
  // 确保数据加载完成且DOM更新后再尝试播放
  nextTick(() => {
    if (videos.value.length > 0 && swiperInstance.value) {
      // 如果 Swiper 初始化时 activeIndex 已经是 0，isActive 会是 true
      // VideoPlayer 会尝试播放
      // 如果需要在 Swiper 初始化后强制播放，可以在数据加载完成后调用 slideTo(0)
      console.log(
        "onSwiper: Videos loaded and swiper instance ready. Active index:",
        swiper.activeIndex
      );
      // The VideoPlayer for index 0 will get isActive=true and handle play
    }
  });
};

// 定义一个加载更多的阈值
const loadMoreThreshold = 5; // 当滑动到倒数第5个视频时开始加载更多
const onSlideChange = (swiper) => {
  console.log("Slide changed to: ", swiper.activeIndex);
  // VideoPlayer 子组件会根据 isActive prop (index === swiper.activeIndex) 自动暂停旧视频并播放新视频

  // 检查是否接近列表底部，触发加载更多
  const totalVideos = videos.value.length;
  const activeIndex = swiper.activeIndex;

  if (activeIndex >= totalVideos - 1 - loadMoreThreshold) {
    console.log(
      `Threshold reached: activeIndex = ${activeIndex}, totalVideos = ${totalVideos}, threshold = ${loadMoreThreshold}`
    );
    fetchData();
  }
};

onMounted(() => {
  fetchData();
});
</script>

<template>
  <div class="tiktok-feed">
    <swiper
      :modules="modules"
      direction="vertical"
      :slides-per-view="1"
      :space-between="0"
      :virtual="true"
      @swiper="onSwiper"
      @slideChange="onSlideChange"
      class="swiper-container"
      no-swiping-class="swiper-no-swiping"
    >
      <swiper-slide
        v-for="(video, index) in videos"
        :key="video.id"
        :virtualIndex="index"
        class="swiper-slide-item"
      >
        <VideoPlayer
          :video="video"
          :index="index"
          :isActive="
            activeTabIndex == props.currentIndex
              ? index === swiperInstance?.activeIndex
              : false
          "
          :randomPlay="props.currentIndex == 2"
        />
      </swiper-slide>
    </swiper>
  </div>
</template>

<style scoped>
.tiktok-feed {
  position: relative;
  width: 100%;
  height: 100%; /* 或者你期望的容器高度 */
  overflow: hidden;
  background-color: #000;
}

.swiper-container {
  width: 100%;
  height: 100%;
}

.swiper-slide-item {
  position: relative; /* 需要 position: relative 让加载提示绝对定位 */
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
  height: 100%;
  background-color: #111; /* 给 slide 一个背景色，防止视频未加载时透明 */
}
</style>
