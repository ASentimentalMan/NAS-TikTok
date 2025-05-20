<script setup>
import { Icon } from "@iconify/vue";
import Loading from "@/components/Loading.vue";
import MusicBase from "@/components/BaseMusic.vue";
import SimpleToast from "@/components/SimpleToast.vue";
import NarrowIcon from "@/assets/imgs/narrow.png";
import RefreshIcon from "@/assets/imgs/refresh.png";
import router from "@/router";
const props = defineProps({
  video: {
    type: Object,
    required: true,
  },
  index: {
    type: Number,
    required: true,
  },
  isActive: {
    // 指示这个视频是否是 Swiper 当前激活的 slide
    type: Boolean,
    default: false,
  },
  randomPlay: {
    type: Boolean,
    default: false,
  },
});
const standalone = inject("standalone");
// 是否第一次播放
let isFirstPlay = true;
/*播放状态*/
const SlideItemPlayStatus = {
  Play: "Play",
  Stop: "Stop",
  Pause: "Pause",
};
const token = computed(() => localStorage.getItem("token"));

const videoRef = ref(null);
const containerRef = ref(null); // **新增：容器元素的引用**
const isPlaying = computed(() => state.status === SlideItemPlayStatus.Play);
const isLoading = ref(true);
const touched = ref(false);
const touchedTimeout = ref(null);
const onTouchStart = () => {
  touched.value = true;
  if (touchedTimeout.value) {
    clearTimeout(touchedTimeout.value);
  }
};
const onTouchEnd = () => {
  touchedTimeout.value = setTimeout(() => {
    touched.value = false;
    touchedTimeout.value = null;
  }, 1000);
};

// 现有的播放状态和进度条状态
const state = reactive({
  duration: 0,
  currentTime: 0,
  step: 0,
  playX: 0,
  progressBarRect: {
    height: 0,
    width: 0,
  },
  isMove: false, // 进度条是否正在拖动
  start: { x: 0 }, // 进度条拖动开始位置
  last: { x: 0, time: 0 }, // 进度条拖动上次位置和时间
  status: SlideItemPlayStatus.Play, // 视频播放状态
});

// 新增的倍速播放相关状态
const speedRef = ref(null); // speed div 的引用
const videoState = reactive({
  longPressTimer: null, // 长按定时器 ID
  isLongPressingSpeed: false, // 是否正在长按 speed 区域
  speedDivRect: { height: 0, width: 0, top: 0, bottom: 0 }, // speed div 的尺寸和位置 (用于计算滑动阈值)
  startY: 0, // 长按开始时的 Y 坐标
  currentY: 0, // 当前触摸移动的 Y 坐标
  isSpeedLocked: false, // 2倍速是否被锁定
  lockToggledThisGesture: false, // 在一次长按滑动中是否已经触发过锁定/解锁
  slidBeyondThreshold: false, // 在长按过程中是否已经滑动超过阈值
});

// **新增：工具按钮相关状态**
const rotationAngle = ref(0); // 视频旋转角度，初始为0
// 静音状态
const muted = inject("muted", {
  type: Boolean,
  default: true,
});

// **新增：用于存储容器和视频原始尺寸的状态**
const containerDimensions = reactive({ width: 0, height: 0 });
const videoOriginalDimensions = reactive({ width: 0, height: 0 });

// **优化：计算缩放比例，适用于任意角度旋转**
const scaleFactor = computed(() => {
  const { width: containerWidth, height: containerHeight } =
    containerDimensions; // 容器尺寸
  const { width: videoWidth, height: videoHeight } = videoOriginalDimensions; // 视频原始尺寸
  const angle = rotationAngle.value; // 原始旋转角度

  // 尺寸无效时返回默认缩放1 (或者 0, 但 1 更安全)
  if (
    containerWidth <= 0 ||
    containerHeight <= 0 ||
    videoWidth <= 0 ||
    videoHeight <= 0
  ) {
    return 1;
  }

  // 将角度转换为弧度
  // 使用 Math.abs 是因为旋转角度的正负不影响边界框大小计算
  const angleRad = (angle % 180) * (Math.PI / 180); // 只需要考虑 0 到 180 度的范围，因为边界框尺寸每180度重复
  const absCos = Math.abs(Math.cos(angleRad));
  const absSin = Math.abs(Math.sin(angleRad));

  // 计算原始视频尺寸在旋转后的最小边界框尺寸
  // 这个边界框是旋转后的视频完全包含在一个矩形内的最小矩形
  const rotatedBoundingBoxWidth = videoWidth * absCos + videoHeight * absSin;
  const rotatedBoundingBoxHeight = videoWidth * absSin + videoHeight * absCos;

  // 计算需要将已旋转视频的边界框缩放到最大以适应容器所需的总缩放比例
  // 这是我们最终需要应用的缩放因子
  const totalScaleNeededToFitRotated = Math.min(
    containerWidth / rotatedBoundingBoxWidth,
    containerHeight / rotatedBoundingBoxHeight
  );

  // 返回计算出的总缩放因子。
  // 这个值会直接用在 CSS transform: scale(...) 中。
  // 注意：如果你的视频元素同时使用了 object-fit: contain，你需要移除它或将其设置为其他值
  // （比如 fill），因为 object-fit 会和 transform: scale 冲突或产生非预期的结果。
  // 最干净的方式是仅通过 transform: scale 来控制缩放。
  return totalScaleNeededToFitRotated;
});

// 计算属性：根据状态生成速度指示器的文本
const speedIndicatorComputedText = computed(() => {
  if (videoState.isLongPressingSpeed) {
    if (videoState.slidBeyondThreshold) {
      // 正在长按，且已滑动超过阈值
      return videoState.isSpeedLocked ? "松手锁定2倍速" : "松手取消2倍速";
    } else {
      // 正在长按，但未滑动超过阈值
      return videoState.isSpeedLocked
        ? "下滑松手取消2倍速"
        : "下滑松手锁定2倍速";
    }
  } else if (videoState.isSpeedLocked) {
    // 不在长按，但已锁定2倍速
    return "2x";
  }
  // 默认不显示（v-if 控制）
  return "";
});

// --- 播放控制方法 ---
const toastRef = ref();
const play = async () => {
  if (videoRef.value) {
    // 注意: 移动端浏览器（尤其是 Safari）对自动播放有限制
    // 通常需要设置 muted=true 或用户交互才能成功播放
    // 处理 play() 返回的 Promise，捕获自动播放失败的情况
    try {
      await videoRef.value.play();
      state.status = SlideItemPlayStatus.Play;
      isLoading.value = false;
    } catch (error) {
      state.status = SlideItemPlayStatus.Stop;
      isLoading.value = false;
      isLoading.value = videoRef.value.readyState < 3; // Check if enough data is buffered to play
      toastRef.value.show("播放被阻止，请点击继续");
    }
  }
};

const pause = () => {
  const videoElement = videoRef.value;
  if (videoElement && !videoElement.paused) {
    state.status = SlideItemPlayStatus.Pause;
    videoElement.pause();
  }
};

const togglePlay = () => {
  // 如果正在长按速度区域，点击不应该暂停/播放视频
  if (videoState.isLongPressingSpeed) {
    return;
  }
  // 如果点击到了工具按钮区域，也不应该触发播放暂停
  // 简单的判断方式是检查事件的 target 是否是 .tool-button 或者其子元素
  if (event.target.closest(".tool-button")) {
    return;
  }

  if (isPlaying.value) {
    pause();
  } else {
    play();
  }
};

// 设置视频播放速度
const setPlaybackRate = (rate) => {
  const videoElement = videoRef.value;
  if (videoElement) {
    videoElement.playbackRate = rate;
    // console.log(`Set playback rate to: ${rate}x for index ${props.index}`);
  }
};

// **新增：工具按钮方法**
// 1. 向左旋转视频
const rotateLeft = () => {
  // 只更新角度值，动画由 CSS transition 处理
  rotationAngle.value -= 90;
};

// 2. 向右旋转视频
const rotateRight = () => {
  // 只更新角度值，动画由 CSS transition 处理
  rotationAngle.value += 90;
};

// **新增：处理旋转动画结束**
const handleTransitionEnd = (event) => {
  // 确保监听的是 transform 属性的 transition 结束
  if (videoRef.value && event.propertyName === "transform") {
    const currentAngle = rotationAngle.value;
    // 检查当前角度是否为 360 度的倍数（考虑正负）
    // 使用一个小的容差，尽管对于 90 度步进通常不是问题
    const normalizedAngle = ((currentAngle % 360) + 360) % 360; // 规范化到 0 到 359.99...

    if (normalizedAngle === 0 && currentAngle !== 0) {
      // 只有当角度是 360 的倍数且不等于 0 时才重置
      // console.log(`Rotation animation ended at ${currentAngle}, resetting to 0.`);

      // 暂时移除 transition 属性，防止反向动画
      videoRef.value.style.transition = "none";

      // 在下一个 DOM 更新周期将角度值设置为 0
      nextTick(() => {
        rotationAngle.value = 0;
        // console.log('rotationAngle reset to 0.');

        // 稍作延迟后恢复 transition 属性
        // 延迟的目的是确保浏览器有足够的时间在没有 transition 的情况下应用新的 0 度值
        // 0毫秒的 setTimeout 配合 nextTick 通常也足够了，或者可以给一个很小的延迟
        setTimeout(() => {
          if (videoRef.value) {
            videoRef.value.style.transition = "transform 0.2s ease-in-out";
            // console.log('Transition re-enabled.');
          }
        }, 50); // 50ms 延迟，可以根据实际效果调整
      });
    }
  }
};

// 3. 随机位置播放
const seekRandom = () => {
  const videoElement = videoRef.value;
  if (videoElement && state.duration > 5) {
    const maxSeekTime = state.duration - 5;
    const randomTime = Math.random() * maxSeekTime;
    videoElement.currentTime = randomTime;
    // 确保更新本地 currentTime 状态以同步 UI
    state.currentTime = Math.floor(randomTime);
    // console.log("Seek Random to:", randomTime);
    play(); // 随机跳转后开始播放
  } else if (videoElement) {
    // console.log("Video too short for random seek (duration < 5s)");
    // 如果视频很短，也可以选择跳转到开头或不操作
    videoElement.currentTime = 0;
    state.currentTime = 0;
    play();
  }
};

// 4. 静音切换
const toggleMute = () => {
  muted.value = !muted.value;
};

// --- Watchers ---
watch(
  () => props.isActive,
  (newValue) => {
    if (newValue) {
      if (props.randomPlay && isFirstPlay) {
        seekRandom();
        isFirstPlay = false;
      } else {
        play();
      }
      // 确保在激活时，如果速度被锁定，保持2x；否则恢复1x
      if (videoState.isSpeedLocked) {
        setPlaybackRate(2);
      } else {
        setPlaybackRate(1);
      }
      // 激活时获取容器和视频原始尺寸，用于计算缩放
      nextTick(() => {
        updateContainerDimensions();
        updateVideoOriginalDimensions(); // 视频原始尺寸可能在 Swiper 切换时才可知
      });
    } else {
      pause();
      // 当视频不可见时，恢复1x速度（除非你希望锁定状态跨Swiper切换保留）
      // 这里选择恢复1x，更符合常见用户习惯
      setPlaybackRate(1);
      // 并且取消任何正在进行的临时2x状态和长按状态
      videoState.isLongPressingSpeed = false;
      videoState.slidBeyondThreshold = false;
      videoState.lockToggledThisGesture = false; // 重置手势状态
      // videoState.isSpeedLocked = false; // 通常切换回来不应该自动解锁，用户手动解锁
      if (videoState.longPressTimer) {
        clearTimeout(videoState.longPressTimer);
        videoState.longPressTimer = null;
      }
      // 非激活状态时，重置旋转角度（确保切换回来时是初始状态）
      rotationAngle.value = 0; // 移除旧的 setTimeout 逻辑后，这里需要确保非激活时归零
    }
  },
  { immediate: true } // 组件挂载时如果 isActive 已经是 true 则立即执行一次
);

// **新增：监听旋转角度变化，强制重新计算尺寸和缩放**
// 虽然 scaleFactor 是计算属性，会响应 rotationAngle 的变化，
// 但确保在旋转后立即更新容器和视频尺寸可能有助于计算准确性，
// 特别是在某些浏览器环境下布局可能需要重新计算。
watch(rotationAngle, () => {
  nextTick(() => {
    updateContainerDimensions();
    // updateVideoOriginalDimensions(); // 视频原始尺寸不会因为旋转而改变
  });
});

const progressRef = ref();
const progressClass = computed(() =>
  state.isMove ? "move" : isPlaying.value ? "" : "stop"
);
const durationStyle = computed(() => ({ width: state.playX + "px" }));
const duration = (v) => {
  if (!v || v < 0) return "00:00";
  const m = Math.floor(v / 60);
  const s = Math.round(v % 60);
  const mm = m < 10 ? "0" + m : "" + m;
  const ss = s < 10 ? "0" + s : "" + s;
  return `${mm}:${ss}`;
};
const stopPropagation = (e) => {
  e.stopImmediatePropagation();
  e.stopPropagation();
  // e.preventDefault(); // 注意：如果在 touchmove 中总是阻止默认事件，可能会影响页面滚动
};

// --- 进度条触摸事件处理 ---
const touchstart = (e) => {
  // 确保在处理进度条事件时，没有在长按速度区域
  if (videoState.isLongPressingSpeed) {
    e.preventDefault(); // 阻止进度条事件，如果速度手势正在进行
    return;
  }
  stopPropagation(e);
  state.isMove = true; // 进入拖动状态
  pause(); // 暂停视频
  state.start.x = e.touches[0].pageX;
  state.last.x = state.playX; // 记录开始拖动时的进度条宽度
  state.last.time = videoRef.value.currentTime; // 记录开始拖动时的视频时间
};
const touchmove = (e) => {
  // 确保在处理进度条事件时，没有在长按速度区域
  if (videoState.isLongPressingSpeed) {
    e.preventDefault(); // 阻止进度条事件，如果速度手势正在进行
    return;
  }
  // stopPropagation(e); // 在 touchmove 中阻止默认事件可能影响页面滚动，按需开启
  if (!state.isMove) return; // 确保只在拖动开始后处理 move
  let dx = e.touches[0].pageX - state.start.x;
  let newPlayX = state.last.x + dx;

  // 限制进度条范围
  if (newPlayX < 0) newPlayX = 0;
  if (newPlayX > state.progressBarRect.width)
    newPlayX = state.progressBarRect.width;

  state.playX = newPlayX;

  // 根据进度条宽度计算新的currentTime
  // 避免除以0
  if (state.progressBarRect.width > 0 && state.duration > 0) {
    state.currentTime =
      state.last.time + (dx / state.progressBarRect.width) * state.duration; // 使用比例计算更准确
  } else {
    state.currentTime = state.last.time; // 无法计算步长时，保持时间不变
  }

  // 限制时间范围
  if (state.currentTime <= 0) state.currentTime = 0;
  if (state.currentTime >= state.duration) state.currentTime = state.duration;

  // 实时更新视频当前时间 (可选，可能会影响性能)
  // videoRef.value.currentTime = state.currentTime;
};
const touchend = (e) => {
  // console.log("progress touchend", e);
  // 确保在处理进度条事件时，没有在长按速度区域
  if (videoState.isLongPressingSpeed) {
    e.preventDefault(); // 阻止进度条事件，如果速度手势正在进行
    return;
  }
  // stopPropagation(e); // touchend 通常不需要阻止默认事件

  // 在 touchend 结束时再设置最终的视频时间
  if (videoRef.value) {
    videoRef.value.currentTime = state.currentTime;
  }

  // 延时退出拖动状态，让时间显示一会儿
  setTimeout(() => {
    state.isMove = false;
    // 恢复播放（如果在 touchend 之前不是暂停状态的话）
    // 或者只在用户点击屏幕中心时才恢复播放，避免拖动后自动播放
    // 这里简单处理，拖动结束就尝试播放
    play();
  }, 500); // 适当的延时，让用户看清时间
};

// --- **新增：倍速触摸事件处理** ---
const longPressDuration = 500; // 长按判定时间 (毫秒)
const slideThresholdRatio = 0.3; // 滑动距离占 speed div 高度的比例（可以调整）

const handleSpeedTouchStart = (e) => {
  // console.log("speed touchstart", e);
  stopPropagation(e); // 阻止事件冒泡和默认行为，防止与Swiper或其他元素冲突
  e.preventDefault(); // 阻止默认行为，非常重要，防止长按触发上下文菜单或滚动

  videoState.startY = e.touches[0].clientY; // 记录触摸开始的Y坐标
  videoState.currentY = e.touches[0].clientY; // 记录当前Y坐标
  videoState.lockToggledThisGesture = false; // 重置本次手势的锁定/解锁状态
  videoState.slidBeyondThreshold = false; // 重置滑动阈值状态

  // 清除任何现有定时器
  if (videoState.longPressTimer) {
    clearTimeout(videoState.longPressTimer);
    videoState.longPressTimer = null;
  }

  // 开始长按定时器
  videoState.longPressTimer = setTimeout(() => {
    videoState.isLongPressingSpeed = true;
    // 长按达到时间，暂时设置为2倍速（如果当前不是锁定状态的话）
    if (!videoState.isSpeedLocked) {
      setPlaybackRate(2);
    }
    // console.log("Long press detected");
  }, longPressDuration);
};

const handleSpeedTouchMove = (e) => {
  // console.log("speed touchmove", e);
  stopPropagation(e); // 阻止事件冒泡和默认行为
  e.preventDefault(); // 阻止默认行为，尤其是滚动

  videoState.currentY = e.touches[0].clientY; // 更新当前Y坐标
  const deltaY = videoState.currentY - videoState.startY; // 计算垂直方向的滑动距离

  // 确保 speedDivRect 已经获取
  if (videoState.speedDivRect.height === 0 && speedRef.value) {
    const rect = speedRef.value.getBoundingClientRect();
    videoState.speedDivRect = {
      height: rect.height,
      width: rect.width,
      top: rect.top,
      bottom: rect.bottom,
    };
    // console.log("Re-calculated Speed div rect:", videoState.speedDivRect);
  }

  // 如果 speedDivRect 仍然不可用，或者不是长按状态，或者本次手势已经切换过锁定状态，则不处理滑动
  if (
    videoState.speedDivRect.height === 0 ||
    !videoState.isLongPressingSpeed ||
    videoState.lockToggledThisGesture
  ) {
    // console.log("Move ignored:", { isLongPressing: videoState.isLongPressingSpeed, lockToggled: videoState.lockToggledThisGesture, rectHeight: videoState.speedDivRect.height });
    return;
  }

  const slideThreshold = videoState.speedDivRect.height * slideThresholdRatio;

  // 判断是否下滑距离超过阈值
  if (deltaY >= slideThreshold) {
    // console.log("Slid beyond threshold");
    videoState.slidBeyondThreshold = true; // 标记已滑动超过阈值

    // 如果这是第一次超过阈值触发锁定/解锁
    if (!videoState.lockToggledThisGesture) {
      videoState.lockToggledThisGesture = true; // 标记本次手势已触发锁定/解锁

      if (videoState.isSpeedLocked) {
        // 如果当前是锁定状态，则解锁，恢复1倍速
        videoState.isSpeedLocked = false;
        setPlaybackRate(1);
        // console.log("Speed unlocked: 1x");
      } else {
        // 如果当前不是锁定状态，则锁定为2倍速
        videoState.isSpeedLocked = true;
        setPlaybackRate(2); // 确保即使长按结束也能保持2倍速
        // console.log("Speed locked: 2x");
      }
      // 可以在这里添加震动反馈或视觉提示
      // navigator.vibrate(50); // 简单的震动反馈（需要用户授权或在特定环境下）
    }
  } else {
    // 滑动距离未超过阈值
    videoState.slidBeyondThreshold = false; // 标记未超过阈值
  }
};

const handleSpeedTouchEnd = (e) => {
  // console.log("speed touchend", e);
  stopPropagation(e); // 阻止事件冒泡和默认行为
  e.preventDefault(); // 阻止默认行为

  // 清除长按定时器
  if (videoState.longPressTimer) {
    clearTimeout(videoState.longPressTimer);
    videoState.longPressTimer = null;
  }

  // 如果是长按结束，且速度没有被锁定，则恢复1倍速
  // 如果速度被锁定，则保持2倍速
  if (videoState.isLongPressingSpeed && !videoState.isSpeedLocked) {
    setPlaybackRate(1);
    // console.log("Long press ended, reverting to 1x");
  }
  // 如果因为滑动已经切换了锁定状态，setPlaybackRate已经在move里处理了

  // 重置长按和滑动相关状态
  videoState.isLongPressingSpeed = false;
  videoState.slidBeyondThreshold = false;
  videoState.lockToggledThisGesture = false; // 重置手势状态以便下次长按可以触发

  //console.log("Touch end processed. isSpeedLocked:", videoState.isSpeedLocked);
};

// **新增：更新容器尺寸**
const updateContainerDimensions = () => {
  if (containerRef.value) {
    const rect = containerRef.value.getBoundingClientRect();
    containerDimensions.width = rect.width;
    containerDimensions.height = rect.height;
    // console.log("Container dimensions updated:", containerDimensions);
  } else {
    console.warn("Container ref is not available.");
  }
};

// **新增：更新视频原始尺寸**
const updateVideoOriginalDimensions = () => {
  const videoElement = videoRef.value;
  if (
    videoElement &&
    videoElement.videoWidth > 0 &&
    videoElement.videoHeight > 0
  ) {
    videoOriginalDimensions.width = videoElement.videoWidth;
    videoOriginalDimensions.height = videoElement.videoHeight;
    // console.log("Video original dimensions updated:", videoOriginalDimensions);
  } else {
    // console.log("Video element not ready or has zero dimensions.");
  }
};

// 使用具名函数，以便在 onUnmounted 中正确移除
const handleLoadedMetadata = () => {
  state.duration = videoRef.value.duration;
  // 确保 progressRef 尺寸已获取
  if (state.progressBarRect.width > 0 && state.duration > 0) {
    state.step = state.progressBarRect.width / state.duration; // 使用 duration 计算步长更准确
    // 根据 currentTime 计算 progress-line 的宽度 (初始 currentTime 通常为 0)
    state.playX =
      (state.currentTime / state.duration) * state.progressBarRect.width;
  } else {
    // 如果尺寸还未获取到，延迟计算或给个默认值
    // console.warn("Progress bar rect not available on loadedmetadata. Delaying step calculation.");
    // 可以考虑在这里再次尝试获取尺寸，或者依赖 timeupdate 时的尺寸
    if (progressRef.value && state.duration > 0) {
      const rect = progressRef.value.getBoundingClientRect();
      state.progressBarRect = rect;
      state.step = rect.width / state.duration;
      state.playX =
        (state.currentTime / state.duration) * state.progressBarRect.width;
    } else {
      state.playX = 0; // 无法计算，设为0
    }
  }

  // 在元数据加载后获取视频原始尺寸
  updateVideoOriginalDimensions();

  // 确保视频加载元数据后，如果 isActive 为 true，就尝试播放
  // 这是处理初始加载的关键
  if (props.isActive) {
    play();
  }
};

// 使用具名函数，以便在 onUnmounted 中正确移除
const handleTimeUpdate = (e) => {
  // 只有当不在拖动进度条时才更新进度条显示
  if (!state.isMove) {
    state.currentTime = Math.floor(e.target.currentTime); // 使用 Math.floor 更符合常规时间显示
    // 确保 currentTime 不超过 duration
    if (state.currentTime > state.duration) state.currentTime = state.duration;
    // 确保 progress bar rect 尺寸已获取，避免除以0
    if (state.progressBarRect.width > 0 && state.duration > 0) {
      // 根据 currentTime 计算 progress-line 的宽度
      state.playX =
        (state.currentTime / state.duration) * state.progressBarRect.width;
      // 避免负值
      if (state.playX < 0) state.playX = 0;
      // 确保 playX 不超过进度条宽度
      if (state.playX > state.progressBarRect.width)
        state.playX = state.progressBarRect.width;
    } else if (progressRef.value && state.duration > 0) {
      // 如果尺寸还未获取到，在 timeupdate 时尝试获取
      const rect = progressRef.value.getBoundingClientRect();
      state.progressBarRect = rect;
      // state.step = rect.width / state.duration; // step 可以在 loadedmetadata 时计算一次就够了
      state.playX =
        (state.currentTime / state.duration) * state.progressBarRect.width;
      if (state.playX < 0) state.playX = 0;
      if (state.playX > state.progressBarRect.width)
        state.playX = state.progressBarRect.width;
    } else {
      // 无法计算，保持当前位置或设为0
      state.playX = 0;
    }
  }
};

// 使用具名函数，以便在 onUnmounted 中正确移除
const handleEnded = () => {
  setPlaybackRate(1); // 播放结束恢复1倍速
  videoState.isSpeedLocked = false; // 解除锁定状态
  videoState.isLongPressingSpeed = false; // 停止长按状态显示
  videoState.slidBeyondThreshold = false; // 重置滑动状态
  videoState.lockToggledThisGesture = false; // 重置手势状态
  state.status = SlideItemPlayStatus.Stop; // 设置为停止状态 (或 Pause)
};

const handleVideoError = async () => {
  try {
    const response = await fetch(`${props.video.url}?auth=${token.value}`, {
      method: "HEAD",
    });
    if (response.status === 401) {
      videoRef.value.pause();
      localStorage.removeItem("token");
      router.replace("/login");
    }
  } catch (error) {
    console.error("检查视频 token 失败:", error);
    videoRef.value.pause();
    localStorage.removeItem("token");
    router.replace("/login");
  }
};

// 使用具名函数，以便在 onUnmounted 中正确移除
const handleWindowResize = () => {
  updateContainerDimensions();
  // 窗口尺寸变化可能影响视频的渲染尺寸，虽然 object-fit 会重新计算，
  // 显式更新视频原始尺寸在这里不是必须的 (它不会变)，但更新容器尺寸是必须的。
  // updateVideoOriginalDimensions(); // 原始尺寸不会因窗口变化而变
  // 确保进度条尺寸也更新
  if (progressRef.value) {
    state.progressBarRect = progressRef.value.getBoundingClientRect();
    // console.log("Progress div rect on resize:", state.progressBarRect);
    // 重新计算 playX based on new width
    if (state.duration > 0) {
      state.playX =
        (state.currentTime / state.duration) * state.progressBarRect.width;
      if (state.playX < 0) state.playX = 0;
      if (state.playX > state.progressBarRect.width)
        state.playX = state.progressBarRect.width;
    } else {
      state.playX = 0; // 无法计算，设为0
    }
  }
  // 确保 speed div 尺寸也更新
  if (speedRef.value) {
    const rect = speedRef.value.getBoundingClientRect();
    videoState.speedDivRect = {
      height: rect.height,
      width: rect.width,
      top: rect.top,
      bottom: rect.bottom,
    };
    // console.log("Speed div rect on resize:", videoState.speedDivRect);
  }
};

// --- Lifecycle Hooks ---
onMounted(() => {
  // 等待 DOM 更新后获取 speed div 和 progress div 的尺寸，以及容器尺寸
  nextTick(() => {
    updateContainerDimensions(); // 获取容器初始尺寸

    if (speedRef.value) {
      const rect = speedRef.value.getBoundingClientRect();
      videoState.speedDivRect = {
        height: rect.height,
        width: rect.width,
        top: rect.top,
        bottom: rect.bottom,
      };
      // console.log("Speed div rect on mount:", videoState.speedDivRect);
    } else {
      console.warn("Speed div not found on mount.");
    }

    if (progressRef.value) {
      state.progressBarRect = progressRef.value.getBoundingClientRect();
      // console.log("Progress div rect on mount:", state.progressBarRect);
    } else {
      console.warn("Progress div not found on mount.");
    }
  });

  // 确保 videoRef.value 在添加事件监听器时可用
  if (videoRef.value) {
    videoRef.value.addEventListener("loadedmetadata", handleLoadedMetadata);
    videoRef.value.addEventListener("timeupdate", handleTimeUpdate);
    videoRef.value.addEventListener("ended", handleEnded);
    videoRef.value.addEventListener("stalled", handleVideoError);
    videoRef.value.addEventListener("transitionend", handleTransitionEnd);
  } else {
    console.warn("Video ref is not available on mount to add event listeners.");
  }

  // 监听窗口 resize 事件
  window.addEventListener("resize", handleWindowResize);
});

onUnmounted(() => {
  pause(); // Ensure video is paused on unmount
  // 清理长按定时器
  if (videoState.longPressTimer) {
    clearTimeout(videoState.longPressTimer);
    videoState.longPressTimer = null;
  }
  // 恢复默认倍速
  setPlaybackRate(1);
  // 重置所有速度相关的状态
  videoState.isSpeedLocked = false;
  videoState.isLongPressingSpeed = false;
  videoState.slidBeyondThreshold = false;
  videoState.lockToggledThisGesture = false;

  // 重置旋转角度
  rotationAngle.value = 0; // 在 unmount 时强制归零，避免状态残留到下一个视频

  // 移除事件监听器
  const videoElement = videoRef.value;
  if (videoElement) {
    videoElement.removeEventListener("loadedmetadata", handleLoadedMetadata);
    videoElement.removeEventListener("timeupdate", handleTimeUpdate);
    videoElement.removeEventListener("ended", handleEnded);
    // **新增：移除 transitionend 监听器**
    videoElement.removeEventListener("transitionend", handleTransitionEnd);
    // 确保移除临时设置的 transition 属性，虽然 unmount 后元素会销毁
    if (videoElement.style.transition === "none") {
      videoElement.style.transition = "transform 0.2s ease-in-out";
    }
  }
  window.removeEventListener("resize", handleWindowResize);
});
</script>

<template>
  <div class="video-player-container" ref="containerRef" @click="togglePlay">
    <video
      ref="videoRef"
      :muted="muted"
      preload="metadata"
      loop
      x5-video-player-type="h5-page"
      :x5-video-player-fullscreen="false"
      webkit-playsinline
      x5-playsinline
      playsinline
      :fullscreen="false"
      :style="{
        transform: `rotate(${rotationAngle}deg) scale(${scaleFactor})`,
      }"
    >
      <source :src="`${video.url}?auth=${token}`" />
      <p>您的浏览器不支持 video 标签。</p>
    </video>
    <Loading v-if="isLoading" style="position: absolute" />
    <Icon
      v-if="!isPlaying && !isLoading"
      icon="fluent:play-28-filled"
      class="pause-icon"
    />
    <div
      ref="speedRef"
      class="swiper-no-swiping speed"
      @touchstart="handleSpeedTouchStart"
      @touchmove="handleSpeedTouchMove"
      @touchend="handleSpeedTouchEnd"
      @touchcancel="handleSpeedTouchEnd"
      @contextmenu.prevent
    >
      <div
        class="speed-indicator"
        v-if="videoState.isLongPressingSpeed || videoState.isSpeedLocked"
      >
        {{ speedIndicatorComputedText }}
      </div>
    </div>
    <div
      class="swiper-no-swiping tools"
      :class="{ active: touched }"
      :style="standalone ? 'bottom: 32px' : ''"
      @touchstart.stop="onTouchStart"
      @touchend.stop="onTouchEnd"
    >
      <img
        class="tool-button"
        style="transform: scaleX(-1)"
        :src="RefreshIcon"
        @click.stop="rotateLeft"
      />
      <img class="tool-button" :src="RefreshIcon" @click.stop="rotateRight" />
      <img class="tool-button" :src="NarrowIcon" @click.stop="seekRandom" />
      <MusicBase
        @click.stop="toggleMute"
        :muted="muted"
        :is-playing="isPlaying"
        :index="props.index"
      />
    </div>
    <div
      v-show="state.duration > 5"
      ref="progressRef"
      class="swiper-no-swiping progress"
      :class="progressClass"
      :style="standalone ? 'bottom: 70px' : ''"
      @click="null"
      @touchstart="touchstart"
      @touchmove="touchmove"
      @touchend="touchend"
    >
      <div class="time" v-if="state.isMove">
        <span class="currentTime">{{ duration(state.currentTime) }}</span>
        <span class="duration"> / {{ duration(state.duration) }}</span>
      </div>
      <template v-if="state.duration > 0 || state.isMove || !isPlaying">
        <div class="bg"></div>
        <div class="progress-line" :style="durationStyle"></div>
        <div class="point"></div>
      </template>
    </div>
    <SimpleToast ref="toastRef" />
  </div>
</template>

<style scoped lang="less">
.video-player-container {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: #000; /* Fallback background */
  overflow: hidden; /* 确保内容不溢出 */
}

.video-player-container video {
  // max-width: 100%;
  // max-height: 100%;
  // object-fit: contain; /* 或根据需要设置为 cover */
  display: block; /* 防止底部出现空白 */
  /*pointer-events: none; /* 防止点击视频本身触发播放/暂停，因为父容器已经处理了 */
  transform-origin: center; /* 确保旋转和缩放是围绕中心进行的 */

  /* **重新启用并调整过渡效果** */
  transition: transform 0.2s ease-in-out; /* 调整为0.2秒，使用 ease-in-out 缓动函数，看起来更自然 */
}

/* 其他样式保持不变 */
.video-overlay {
  position: absolute;
  bottom: 20px;
  left: 20px;
  color: white;
  background-color: rgba(0, 0, 0, 0.5);
  padding: 10px;
  border-radius: 5px;
  z-index: 10; /* 确保叠加层在视频上方 */
}

.video-overlay p {
  margin: 5px 0;
  font-size: 0.9em;
}

.video-overlay button {
  margin-top: 10px;
  padding: 5px 10px;
  cursor: pointer;
}

.progress {
  // background: green;
  z-index: 100;
  @w: 90%;
  position: absolute;
  bottom: 42px; // 可以根据需要调整位置，避免和速度指示器重叠
  height: 10px;
  left: calc((100% - @w) / 2);
  width: @w;
  display: flex;
  align-items: flex-end;
  padding: 12px 0;

  .time {
    position: absolute;
    z-index: 9;
    font-size: 24px;
    bottom: 50px;
    left: 0;
    right: 0;
    color: white;
    text-align: center;

    .duration {
      color: darkgray;
    }
  }

  @radius: 10px;

  @h: 2px;
  @tr: height 0.3s;

  .bg {
    transition: @tr;
    position: absolute;
    width: 100%;
    height: @h;
    background: #4f4f4f;
    border-radius: @radius;
  }

  @p: 50px;

  .progress-line {
    transition: @tr;
    height: calc(@h + 0.5px);
    width: @p; /* 初始宽度会被计算覆盖 */
    border-radius: @radius 0 0 @radius;
    background: #777777;
    z-index: 1;
  }

  .point {
    transition: all 0.2s;
    width: @h+2;
    height: @h+2;
    border-radius: 50%;
    background: gray;
    z-index: 2;
    /* 调整 transform 使点居中于进度条末端 */
    transform: translate(0, 1px); /* -50% 相对自身宽度 */
    // margin-left: v-bind('state.playX + "px"'); /* 根据 playX 定位 */
  }
}
& .move {
  @h: 10px;

  .bg {
    height: @h;
    background: rgb(31, 37, 52);
  }

  .progress-line {
    height: @h;
    background: rgb(186, 186, 187);
  }

  .point {
    width: @h+2;
    height: @h+2;
    background: white;
  }
}
& .stop {
  @h: 4px;

  .bg {
    height: @h;
  }

  .progress-line {
    height: @h;
    background: white;
  }

  .point {
    width: @h+2;
    height: @h+2;
    background: white;
  }
}

/* 速度检测区域 */
.speed {
  width: 76px; /* 示例宽度，可以调整 */
  height: 70%; /* 让整个高度都可以触发长按 */
  /* background: rgba(255, 0, 0, 0.1); // 半透明红色方便调试，实际使用时移除 */
  position: absolute;
  left: 0; /* 靠左侧作为速度触摸区域 */
  bottom: 12%;
  z-index: 100; /* 确保在视频上方，但不高于指示器 */
  // background: red;
}

/* 速度指示器（文本显示） */
.speed-indicator {
  position: fixed; /* 或者 absolute，取决于你想让它相对于哪里定位 */
  bottom: 30px; /* 距离底部适当的距离，避免和进度条重叠 */
  left: 50%;
  transform: translateX(-50%); /* 只沿X轴居中 */
  color: white;
  font-size: 12px; /* 可以调整字体大小 */
  // background-color: rgba(0, 0, 0, 0.7); /* 深色半透明背景 */
  // padding: 8px 15px; /* 调整内边距 */
  border-radius: 20px; /* 圆角胶囊状 */
  pointer-events: none; /* 确保指示器本身不影响触摸事件 */
  white-space: nowrap; /* 防止文本换行 */
  z-index: 101; /* 确保在其他叠加层和速度触摸区域上方 */
}

/* 工具按钮区域 */
.tools {
  position: absolute;
  right: 10px; /* 靠右侧，可以调整距离 */
  bottom: 0%;
  transform: translateY(-40%); /* 垂直居中 */
  z-index: 100; /* 确保在视频上方 */
  // background-color: red; /* 移除调试背景色 */

  display: flex; // 启用 Flexbox 布局
  flex-direction: column; // 竖向排列子元素
  align-items: center; // 子元素在交叉轴（水平方向）居中
  gap: 15px; // 子元素之间的间距，可以调整
  opacity: 0;
  transition: opacity 0.4s ease-in-out;
  .tool-button {
    width: 36px; /* 按钮大小 */
    height: 36px;
    cursor: pointer;
    flex-shrink: 0; // 防止按钮被挤压
  }
}
& .active {
  opacity: 1;
  transition: opacity 0.4s ease-in-out;
}

.loading-indicator,
.play-button-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  color: white;
  font-size: 2em;
  pointer-events: none; /* 默认不阻止点击 */
  z-index: 1;
}

.pause-icon {
  width: 60px;
  height: 60px;
  opacity: 0.3;
  position: absolute;
  margin: auto;
  left: 0;
  top: 0;
  bottom: 0;
  right: 0;
  color: white;
  animation: pause-animation 1.1s linear;

  @scale: scale(1.2);

  @keyframes pause-animation {
    0% {
      opacity: 0;
      transform: scale(2);
    }
    10% {
      opacity: 0.3;
      transform: @scale;
    }
    100% {
      transform: @scale;
      opacity: 0.3;
    }
  }
}
</style>
