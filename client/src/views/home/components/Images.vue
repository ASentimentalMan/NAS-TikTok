<script setup>
import { ref, onMounted, nextTick, watch } from "vue"; // 引入 ref, watch
// 引入 Swiper Vue 组件
import { Swiper, SwiperSlide } from "swiper/vue";
// 引入 Swiper 核心样式和所需模块的样式
import "swiper/css";
import "swiper/css/virtual"; // 虚拟模块样式
// 引入 Autoplay 模块的样式 (通常不需要单独导入autoplay的css)
// import 'swiper/css/autoplay';

// 引入 Swiper 模块
import { Virtual, Autoplay } from "swiper/modules"; // <-- 引入 Autoplay 模块

// 引入 ImagePlayer 组件
import ImagePlayer from "@/components/ImagePlayer.vue";
import { fetchImageList } from "@/apis/index";

const props = defineProps({
  currentIndex: {
    type: Number,
    default: 0,
  },
});

// 模块包含 Virtual 和 Autoplay
const modules = [Virtual, Autoplay]; // <-- 在模块数组中加入 Autoplay

// 视频数据
const images = ref([]);

const swiperInstance = ref(null);

// --- 控制自动播放的状态变量 ---
const isAutoplayEnabled = ref(false); // <-- 新增：控制自动播放是否开启的状态
provide("isAutoplayEnabled", isAutoplayEnabled);

// 切换自动播放状态的方法
const toggleAutoplay = () => {
  isAutoplayEnabled.value = !isAutoplayEnabled.value;
};
provide("toggleAutoplay", toggleAutoplay);

const handleAutoplayStop = () => {
  console.log("Swiper autoplay stopped internally.");
  // 当 Swiper 自动播放停止时（例如，因为用户交互且 disableOnInteraction: true），
  // 更新我们的状态变量以保持同步
  if (isAutoplayEnabled.value) {
    console.log("Syncing state: isAutoplayEnabled set to false.");
    isAutoplayEnabled.value = false;
  }
};

// 监听 isAutoplayEnabled 的变化，手动控制 Swiper 的自动播放
watch(isAutoplayEnabled, (newValue) => {
  if (swiperInstance.value) {
    if (newValue) {
      console.log("Starting autoplay");
      swiperInstance.value.autoplay.start();
    } else {
      console.log("Stopping autoplay");
      swiperInstance.value.autoplay.stop();
    }
  }
});

// 加载视频数据的方法
const fetchData = async () => {
  try {
    const response = await fetchImageList();
    const list = response.data;
    if (list && list.length > 0) {
      // Use a Set to filter out potential duplicates if your API might return them
      const existingIds = new Set(images.value.map((img) => img.id));
      const newList = list.filter((img) => !existingIds.has(img.id));

      if (newList.length > 0) {
        images.value = images.value.concat(newList);
        console.log("More data loaded, total images:", images.value.length);

        // 通知 Swiper Virtual 模块数据已更新
        if (swiperInstance.value && swiperInstance.value.virtual) {
          // virtual.update(true) is crucial after adding more data
          swiperInstance.value.virtual.update(true);
          console.log("Swiper virtual updated");
        }
      } else {
        console.log("No new data to load from the latest fetch.");
      }
    } else {
      console.log("No more data to load from API.");
    }
  } catch (error) {
    console.error("Error loading more images:", error);
  }
};

// --- Swiper 事件和视频控制 ---
const onSwiper = (swiper) => {
  swiperInstance.value = swiper;
  console.log("Swiper initialized");
  // 可以在这里做一些初始化操作
  // 第一个视频的播放由 ImagePlayer 组件自身的 onMounted 和 isActive 监听器处理
  // 确保数据加载完成且DOM更新后再尝试播放
  nextTick(() => {
    if (images.value.length > 0 && swiperInstance.value) {
      console.log(
        "onSwiper: images loaded and swiper instance ready. Active index:",
        swiper.activeIndex
      );
      // The ImagePlayer for index 0 will get isActive=true and handle display/playback

      // 根据初始状态启动/停止自动播放
      // if (isAutoplayEnabled.value) {
      //   swiperInstance.value.autoplay.start();
      // } else {
      //   swiperInstance.value.autoplay.stop();
      // }
    }
  });
};

// 定义一个加载更多的阈值
const loadMoreThreshold = 5; // 当滑动到倒数第5个视频时开始加载更多
const onSlideChange = (swiper) => {
  console.log("Slide changed to: ", swiper.activeIndex);
  // ImagePlayer 子组件会根据 isActive prop (index === swiper.activeIndex) 自动暂停旧视频并播放新视频

  // 检查是否接近列表底部，触发加载更多
  const totalImages = images.value.length;
  const activeIndex = swiper.activeIndex;

  // 确保在有数据且接近末尾时才触发加载
  if (totalImages > 0 && activeIndex >= totalImages - 1 - loadMoreThreshold) {
    console.log(
      `Threshold reached: activeIndex = ${activeIndex}, totalImages = ${totalImages}, threshold = ${loadMoreThreshold}`
    );
    fetchData(); // Trigger loading more data
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
      @autoplayStop="handleAutoplayStop"
      class="swiper-container"
      no-swiping-class="swiper-no-swiping"
      :autoplay="
        isAutoplayEnabled
          ? {
              // <-- 根据状态动态绑定 autoplay
              delay: 2000, // 自动切换的时间间隔（单位ms），这里设置为 5 秒
              disableOnInteraction: true, // 用户操作后是否停止自动播放，false 表示不停止。如果 isAutoplayEnabled 为 false，则将 autoplay 设置为 false
            }
          : false
      "
    >
      <swiper-slide
        v-for="(image, index) in images"
        :key="image.id"
        :virtualIndex="index"
        class="swiper-slide-item"
      >
        <ImagePlayer
          :image="image"
          :isActive="index === swiperInstance?.activeIndex"
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

/* 按钮样式 */
.autoplay-toggle-button {
  position: absolute;
  bottom: 20px; /* 距离底部的位置 */
  left: 50%;
  transform: translateX(-50%);
  z-index: 10; /* 确保按钮在 Swiper 上层 */
  padding: 10px 20px;
  background-color: rgba(0, 0, 0, 0.5);
  color: white;
  border: none;
  border-radius: 5px;
  cursor: pointer;
  font-size: 16px;
}

.autoplay-toggle-button:hover {
  background-color: rgba(0, 0, 0, 0.7);
}
</style>
