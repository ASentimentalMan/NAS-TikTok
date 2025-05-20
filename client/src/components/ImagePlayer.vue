<script setup>
import { Icon } from "@iconify/vue";
import RefreshIcon from "@/assets/imgs/refresh.png";
const router = useRouter();

const props = defineProps({
  image: {
    type: Object,
    required: true,
  },
});
const token = computed(() => localStorage.getItem("token"));

const imageRef = ref(null);
const containerRef = ref(null);

// 工具按钮相关状态
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

const rotationAngle = ref(0);

// 用于存储容器和图片原始尺寸的状态
const containerDimensions = reactive({ width: 0, height: 0 });
const imageOriginalDimensions = reactive({ width: 0, height: 0 });

// 缩放和位置状态
const isZoomed = ref(false); // 是否处于放大状态 (双击放大或双指缩放大于初始fit scale)
const zoomScale = ref(1); // 当前的额外缩放倍率 (1 表示初始fit，大于1表示放大)
const imageTranslateX = ref(0); // 图片在 X 轴上的平移量 (px)
const imageTranslateY = ref(0); // 图片在 Y 轴上的平移量 (px)

// 拖拽相关状态
const isDragging = ref(false); // 是否正在进行实际的拖拽移动
const isPotentialDrag = ref(false); // 是否处于潜在的拖拽开始状态 (按下未移动)
const dragStartX = ref(0); // 拖拽开始时的鼠标/触摸点 X 坐标 (单指)
const dragStartY = ref(0); // 拖拽开始时的鼠标/触摸点 Y 坐标 (单指)
const startTranslateX = ref(0); // 拖拽开始前图片的 imageTranslateX
const startTranslateY = ref(0); // 拖拽开始前图片的 imageTranslateY
const dragThreshold = 5; // 判定为拖拽所需的最小移动距离 (像素)

// 双指缩放相关状态
const isPinching = ref(false); // 是否处于双指缩放状态
const initialPinchDistance = ref(0); // 双指缩放开始时，两个触摸点之间的距离
const initialPinchScale = ref(1); // 双指缩放开始时，当前的 zoomScale 值
const initialPinchMidpoint = reactive({ x: 0, y: 0 }); // 双指缩放开始时，两个触摸点的中心点 (相对于容器)
const initialImageTranslate = reactive({ x: 0, y: 0 }); // 双指缩放开始时，图片的 imageTranslateX/Y

// 缩放限制
const minZoomScale = 1; // 最小额外缩放倍率 (1 表示适配容器)
const maxZoomScale = 10; // 最大额外缩放倍率 (相对于适配容器的倍数)

// 用于双击检测
const lastTapTime = ref(0);

// 计算初始适应容器的缩放比例，适用于任意角度旋转
const initialFitScale = computed(() => {
  const { width: containerWidth, height: containerHeight } =
    containerDimensions;
  const { width: videoWidth, height: videoHeight } = imageOriginalDimensions;
  const angle = rotationAngle.value;

  if (
    containerWidth <= 0 ||
    containerHeight <= 0 ||
    videoWidth <= 0 ||
    videoHeight <= 0
  ) {
    return 1;
  }

  const angleRad = (angle % 180) * (Math.PI / 180);
  const absCos = Math.abs(Math.cos(angleRad));
  const absSin = Math.abs(Math.sin(angleRad));

  const rotatedBoundingBoxWidth = videoWidth * absCos + videoHeight * absSin;
  const rotatedBoundingBoxHeight = videoWidth * absSin + videoHeight * absCos;

  if (
    rotatedBoundingBoxWidth <= containerWidth &&
    rotatedBoundingBoxHeight <= containerHeight
  ) {
    // 如果旋转后的图片完全适合容器，则按原始图片尺寸计算适配比例
    return Math.min(containerWidth / videoWidth, containerHeight / videoHeight);
  }

  // 否则，按旋转后的边界框计算适配比例
  const totalScaleNeededToFitRotated = Math.min(
    containerWidth / rotatedBoundingBoxWidth,
    containerHeight / rotatedBoundingBoxHeight
  );

  return totalScaleNeededToFitRotated;
});

// 计算当前实际应用的缩放比例 (total scale)
const currentAppliedScale = computed(() => {
  return initialFitScale.value * zoomScale.value;
});

// 计算放大状态下的最大平移范围
const maxTranslate = computed(() => {
  const { width: containerWidth, height: containerHeight } =
    containerDimensions;
  const { width: videoWidth, height: videoHeight } = imageOriginalDimensions;
  const currentAngle = rotationAngle.value;
  const currentScale = currentAppliedScale.value;

  if (
    containerWidth <= 0 ||
    containerHeight <= 0 ||
    videoWidth <= 0 ||
    videoHeight <= 0 ||
    currentScale <= initialFitScale.value // 小于等于初始适配比例时，不允许平移
  ) {
    return { x: 0, y: 0 };
  }

  // 计算当前缩放和旋转后的图片在容器中的实际占据尺寸 (基于原始图片尺寸和 total scale)
  const angleRad = (currentAngle % 180) * (Math.PI / 180);
  const absCos = Math.abs(Math.cos(angleRad));
  const absSin = Math.abs(Math.sin(angleRad));

  // Calculate the actual dimensions of the *scaled* and *rotated* image's bounding box
  const scaledRotatedWidth =
    videoWidth * currentScale * absCos + videoHeight * currentScale * absSin;
  const scaledRotatedHeight =
    videoWidth * currentScale * absSin + videoHeight * currentScale * absCos;

  // 最大平移距离是图片超出容器的一半
  const maxTx = Math.max(0, (scaledRotatedWidth - containerWidth) / 2);
  const maxTy = Math.max(0, (scaledRotatedHeight - containerHeight) / 2);

  return {
    x: maxTx,
    y: maxTy,
  };
});

// 限制当前平移量在允许的最大范围内
const clampCurrentTranslate = () => {
  const maxTx = maxTranslate.value.x;
  const maxTy = maxTranslate.value.y;

  // Add a check if container/image dimensions are valid before clamping
  if (
    containerDimensions.width <= 0 ||
    containerDimensions.height <= 0 ||
    imageOriginalDimensions.width <= 0 ||
    imageOriginalDimensions.height <= 0
  ) {
    // Cannot clamp if dimensions are unknown or zero
    return;
  }

  if (
    !isNaN(imageTranslateX.value) &&
    !isNaN(imageTranslateY.value) &&
    !isNaN(maxTx) &&
    !isNaN(maxTy) &&
    isFinite(imageTranslateX.value) &&
    isFinite(imageTranslateY.value) &&
    isFinite(maxTx) &&
    isFinite(maxTy)
  ) {
    imageTranslateX.value = Math.max(
      -maxTx,
      Math.min(maxTx, imageTranslateX.value)
    );
    imageTranslateY.value = Math.max(
      -maxTy,
      Math.min(maxTy, imageTranslateY.value)
    );
  } else {
    console.warn("Invalid translate or maxTranslate values, skipping clamp.", {
      tx: imageTranslateX.value,
      ty: imageTranslateY.value,
      maxTx,
      maxTy,
    });
    // Optionally reset to origin if values are invalid
    // imageTranslateX.value = 0;
    // imageTranslateY.value = 0;
  }
};

// 向左旋转图片方法
const rotateLeft = () => {
  // 如果处于放大状态，先重置缩放和位置，再旋转
  if (isZoomed.value) {
    resetZoom();
    // 使用 nextTick 确保重置状态更新后，再改变旋转角度
    nextTick(() => {
      rotationAngle.value -= 90;
    });
  } else {
    // 未放大则直接旋转
    rotationAngle.value -= 90;
  }
};

// 向右旋转图片方法
const rotateRight = () => {
  // 如果处于放大状态，先重置缩放和位置，再旋转
  if (isZoomed.value) {
    resetZoom();
    // 使用 nextTick 确保重置状态更新后，再改变旋转角度
    nextTick(() => {
      rotationAngle.value += 90;
    });
  } else {
    // 未放大则直接旋转
    rotationAngle.value += 90;
  }
};
// 自动播放
const isAutoplayEnabled = inject("isAutoplayEnabled");
const toggleAutoplay = inject("toggleAutoplay");

// 处理旋转动画结束 (修改此函数，移除 360 度归零的逻辑)
const handleTransitionEnd = (event) => {
  if (imageRef.value && event.propertyName === "transform") {
    // The browser handles the rotation correctly for angles > 360.
    // We don't need to manually reset rotationAngle to 0 here
    // when it reaches a multiple of 360.
    // Removing the logic that checks normalizedAngle and resets rotationAngle.value to 0.
    // However, we still need to ensure the transition style is correctly applied after manual gestures end.
    // The transition style is now controlled by the inline style binding based on gesture states.
    // So, this transitionend handler might not be strictly necessary for transition control anymore,
    // but we can keep it for potential future debugging or complex animation chaining.
    // The original logic for re-enabling transition after resetting it for manual control is now handled
    // by the computed transition style based on isDragging, isPotentialDrag, isPinching.
  }
};

// 监听旋转角度变化，强制重新计算尺寸和缩放
watch(rotationAngle, () => {
  nextTick(() => {
    updateContainerDimensions();
    // 旋转后需要重新计算平移量，通常重置平移量是比较好的用户体验
    // 如果之前是放大的，旋转后可能需要调整缩放或重置
    if (isZoomed.value) {
      // 简单的处理是重置放大状态，让它重新计算fit scale
      resetZoom(); // 重置会触发watch(isZoomed)或watch(rotationAngle)导致重新适配
    } else {
      // 如果未放大，确保平移量是0
      imageTranslateX.value = 0;
      imageTranslateY.value = 0;
    }
  });
});

// 监听缩放状态变化，更新 isZoomed 状态 (当 zoomScale 大于 1 或双指缩放引起放大时)
watch(zoomScale, (newScale) => {
  // 判定是否处于放大状态的逻辑可以更复杂，这里简单地认为 zoomScale > 1 就是放大了
  // 也可以根据 currentAppliedScale > initialFitScale 来判断
  isZoomed.value = newScale > 1;
  // 当 zoomScale 变化时，平移范围 maxTranslate 会变化，需要重新限制当前平移量
  nextTick(() => {
    clampCurrentTranslate();
  });
});

// 更新容器尺寸
const updateContainerDimensions = () => {
  if (containerRef.value) {
    const rect = containerRef.value.getBoundingClientRect();
    containerDimensions.width = rect.width;
    containerDimensions.height = rect.height;
    // 容器尺寸变化可能影响 maxTranslate，需要重新限制平移
    clampCurrentTranslate();
  } else {
    // console.warn("Container ref is not available.");
  }
};

// 更新图片原始尺寸
const updateimageOriginalDimensions = () => {
  const imageElement = imageRef.value;
  if (
    imageElement &&
    imageElement.naturalWidth > 0 &&
    imageElement.naturalHeight > 0
  ) {
    imageOriginalDimensions.width = imageElement.naturalWidth;
    imageOriginalDimensions.height = imageElement.naturalHeight;
    // 图片原始尺寸变化可能影响 initialFitScale 和 maxTranslate，需要重新计算和限制平移
    // initialFitScale 是 computed property 会自动更新
    clampCurrentTranslate(); // Re-clamp based on potentially new maxTranslate
  } else {
    // console.log("Image element not ready or has zero dimensions.");
  }
};

// 窗口尺寸变化处理
const handleWindowResize = () => {
  updateContainerDimensions();
  // 当窗口尺寸改变时，容器尺寸会变，图片的适配比例和最大平移范围都会变
  // 如果处于放大状态，简单起见，重置到适配状态是比较好的用户体验
  // 复杂的实现需要根据新的尺寸和当前的缩放中心重新计算缩放和位置
  if (isZoomed.value || isPinching.value) {
    // Resize during zoom or pinch -> reset
    resetZoom();
  } else {
    // 如果未放大，确保平移量是0，并且重新计算适配比例（通过 updateContainerDimensions 触发 initialFitScale 更新）
    imageTranslateX.value = 0;
    imageTranslateY.value = 0;
  }
  // 省略了进度条和速度div相关的代码
};

// 图片加载完成处理
const onImageload = () => {
  updateContainerDimensions();
  updateimageOriginalDimensions();
  // 图片加载完成后，确保重置缩放和位置
  resetZoom(); // Start with the image fitted
};

// 图片加载错误处理
const handleImageError = async () => {
  try {
    const response = await fetch(`${props.image.url}?auth=${token.value}`, {
      method: "HEAD",
    });
    if (response.status === 401) {
      localStorage.removeItem("token");
      router.replace("/login");
    }
  } catch (error) {
    console.error("检查图片 token 失败:", error);
  }
};

// 处理双击 (或双击模拟) - 此函数现在仅由 handleClick 调用
const handleZoomToggle = (event) => {
  const container = containerRef.value;
  const image = imageRef.value;
  if (!container || !image) return;

  const containerRect = container.getBoundingClientRect();

  if (zoomScale.value <= 1) {
    // 如果当前是适配状态 (或小于等于1)，双击放大

    // 1. 计算点击点相对于容器左上角的偏移量
    const clickXRelativeToContainerTopLeft = event
      ? event.clientX - containerRect.left
      : containerRect.width / 2; // 如果没有事件，默认为中心
    const clickYRelativeToContainerTopLeft = event
      ? event.clientY - containerRect.top
      : containerRect.height / 2; // 如果没有事件，默认为中心

    // 2. 计算点击点相对于【容器中心】的偏移量
    const clickXRelativeToContainerCenter =
      clickXRelativeToContainerTopLeft - containerRect.width / 2;
    const clickYRelativeToContainerCenter =
      clickYRelativeToContainerTopLeft - containerRect.height / 2;

    // 3. 计算缩放比例
    const targetZoomScale = 2; // 你想要的放大倍率，可以调整
    const currentTotalScale = currentAppliedScale.value; // 当前总缩放 (通常是 initialFitScale)
    const targetTotalScale = initialFitScale.value * targetZoomScale; // 目标总缩放
    const scaleRatio = targetTotalScale / currentTotalScale; // 本次操作的缩放比例

    // 4. 计算新的平移量（相对于容器中心）
    // 公式: newTranslate = oldTranslate * scaleRatio - clickPointRelativeToCenter * (scaleRatio - 1)
    // oldTranslate 是当前的 imageTranslateX/Y
    const newTranslateX =
      imageTranslateX.value * scaleRatio -
      clickXRelativeToContainerCenter * (scaleRatio - 1);
    const newTranslateY =
      imageTranslateY.value * scaleRatio -
      clickYRelativeToContainerCenter * (scaleRatio - 1);

    // 5. 应用放大和计算出的平移
    zoomScale.value = targetZoomScale;

    nextTick(() => {
      imageTranslateX.value = newTranslateX;
      imageTranslateY.value = newTranslateY;
      clampCurrentTranslate(); // 立即约束平移量
    });
  } else {
    // 双击缩小，重置到初始适配状态
    resetZoom();
  }
};

// 重置缩放和位置到初始适配容器的状态
const resetZoom = () => {
  imageTranslateX.value = 0;
  imageTranslateY.value = 0;
  zoomScale.value = 1; // 将额外缩放倍率重置为1，实际缩放将是 initialFitScale * 1
  // isZoomed.value = false; // Handled by watch(zoomScale)
  // 重置双击计时，避免重置后立即误判为双击
  lastTapTime.value = 0;
  // 重置 Pinch 状态
  isPinching.value = false;
  initialPinchDistance.value = 0;
  initialPinchScale.value = 1;
  initialPinchMidpoint.x = 0;
  initialPinchMidpoint.y = 0;
  initialImageTranslate.x = 0;
  initialImageTranslate.y = 0;
  // 重置 Drag 状态
  isDragging.value = false;
  isPotentialDrag.value = false;
  dragStartX.value = 0;
  dragStartY.value = 0;
  startTranslateX.value = 0;
  startTranslateY.value = 0;

  // 确保过渡动画在重置后恢复
  if (imageRef.value) {
    // Transition style is now controlled by the inline style binding
    // imageRef.value.style.transition = "transform 0.2s ease-in-out";
  }
};

// 处理 click 事件，用于模拟双击
const handleClick = (event) => {
  // 如果处于潜在拖拽或实际拖拽状态，不处理点击事件
  if (isPotentialDrag.value || isDragging.value) {
    // This click event is a consequence of a potential drag that didn't cross the threshold, or shouldn't happen after a full drag anyway.
    // Clear potential drag state and ignore this click as it might interfere with tap detection.
    isPotentialDrag.value = false;
    lastTapTime.value = 0; // Ensure no accidental double tap if a subsequent tap happens
    // Prevent default click behavior which might interfere with other interactions
    // event.preventDefault(); // Removed this line as preventing default here might break double-tap detection on some browsers
    // event.stopPropagation(); // Consider if you need to stop propagation here
    return; // Ignore this click
  }

  const now = new Date().getTime();
  const timeDiff = now - lastTapTime.value;
  const doubleTapThreshold = 300; // 双击时间阈值 (毫秒)

  // 检查是否是双击
  if (timeDiff < doubleTapThreshold && timeDiff > 0) {
    // 检测到双击
    handleZoomToggle(event);
    lastTapTime.value = 0; // 双击后重置时间，避免三次点击被误判
    event.preventDefault(); // 阻止默认行为，如移动端浏览器自带的放大/缩小
    event.stopPropagation(); // 阻止事件冒泡到父元素，避免其他可能绑定的点击/双击事件被触发
  } else {
    // 这是第一次点击，记录时间
    lastTapTime.value = now;
  }
};

// 处理容器的 pointerdown/mousedown/touchstart 事件
const handlePointerDown = (event) => {
  // 如果是鼠标右键或中键，忽略
  if (event.button && event.button !== 0) {
    return;
  }

  // --- Touch Events ---
  if (event.type.startsWith("touch")) {
    if (event.touches.length === 2) {
      // Detected 2 fingers: Start pinching
      isPinching.value = true;
      isPotentialDrag.value = false; // Cancel any potential drag
      isDragging.value = false; // Cancel any ongoing drag
      lastTapTime.value = 0; // Cancel any pending tap detection

      const containerRect = containerRef.value.getBoundingClientRect();
      initialPinchDistance.value = Math.sqrt(
        Math.pow(event.touches[1].clientX - event.touches[0].clientX, 2) +
          Math.pow(event.touches[1].clientY - event.touches[0].clientY, 2)
      );
      initialPinchScale.value = zoomScale.value; // Store current zoomScale
      initialPinchMidpoint.x =
        (event.touches[0].clientX + event.touches[1].clientX) / 2 -
        containerRect.left;
      initialPinchMidpoint.y =
        (event.touches[0].clientY + event.touches[1].clientY) / 2 -
        containerRect.top;
      initialImageTranslate.x = imageTranslateX.value;
      initialImageTranslate.y = imageTranslateY.value;

      // Prevent default browser zoom/pan for 2-finger touch
      event.preventDefault();

      // Add global listeners for subsequent touch events
      window.addEventListener("touchmove", handlePointerMove, {
        passive: false,
      }); // passive: false is crucial
      window.addEventListener("touchend", handlePointerUp);
      window.addEventListener("touchcancel", handlePointerUp);
    } else if (event.touches.length === 1) {
      // Detected 1 finger: Start potential drag (only if not already pinching)
      if (!isPinching.value) {
        // Check for potential double tap before starting drag
        // (handleClick handles the actual double tap detection on touchend/click)
        // We still need to start potential drag here to track movement threshold

        isPotentialDrag.value = true;
        dragStartX.value = event.touches[0].clientX;
        dragStartY.value = event.touches[0].clientY;
        startTranslateX.value = imageTranslateX.value;
        startTranslateY.value = imageTranslateY.value;

        // Add global listeners
        window.addEventListener("touchmove", handlePointerMove, {
          passive: false,
        }); // passive: false for drag too
        window.addEventListener("touchend", handlePointerUp);
        window.addEventListener("touchcancel", handlePointerUp);

        // Do NOT prevent default here, let click event potentially fire for double tap
        // event.preventDefault(); // REMOVED
      }
    }
  }
  // --- Mouse Events ---
  else {
    // Mouse down: Start potential drag (simulating 1-finger touch)
    // Only start drag if not currently pinching (shouldn't happen with mouse, but defensive)
    if (!isPinching.value) {
      // Check for potential double click (handleClick handles actual detection)

      isPotentialDrag.value = true;
      dragStartX.value = event.clientX;
      dragStartY.value = event.clientY;
      startTranslateX.value = imageTranslateX.value;
      startTranslateY.value = imageTranslateY.value;

      // Add global listeners
      window.addEventListener("mousemove", handlePointerMove);
      window.addEventListener("mouseup", handlePointerUp);

      // Do NOT prevent default here, let click event potentially fire for double click
      // event.preventDefault(); // REMOVED
    }
  }
};

// 处理全局的 pointermove/mousemove/touchmove 事件
const handlePointerMove = (event) => {
  // Prevent default browser behavior for touchmove if we are actively handling gestures
  if (event.type.startsWith("touch")) {
    if (isPinching.value || isDragging.value || isPotentialDrag.value) {
      event.preventDefault(); // Crucial to prevent scrolling/zooming
    }
    if (event.touches.length === 0) {
      // Should not happen frequently, but handle gracefully
      return;
    }
  }
  // For mousemove, only prevent default if dragging (to prevent selection)
  else if (isDragging.value) {
    event.preventDefault();
  }

  // --- Pinch Logic (High Priority) ---
  if (
    isPinching.value &&
    event.type.startsWith("touch") &&
    event.touches.length === 2
  ) {
    const currentDistance = Math.sqrt(
      Math.pow(event.touches[1].clientX - event.touches[0].clientX, 2) +
        Math.pow(event.touches[1].clientY - event.touches[0].clientY, 2)
    );

    if (initialPinchDistance.value === 0) {
      // Avoid division by zero if fingers started at the exact same point
      return;
    }

    const scaleRatio = currentDistance / initialPinchDistance.value;

    // Calculate proposed new total scale
    const initialTotalScale = initialFitScale.value * initialPinchScale.value;
    const proposedTotalScale = initialTotalScale * scaleRatio;

    // Clamp the total scale between minZoomScale and maxZoomScale relative to initialFitScale
    const clampedTotalScale = Math.max(
      initialFitScale.value * minZoomScale,
      Math.min(initialFitScale.value * maxZoomScale, proposedTotalScale)
    );

    // Calculate the actual scale ratio applied after clamping, relative to the state at pinch start
    const actualScaleRatio = clampedTotalScale / initialTotalScale;

    // **重要修改：更新 zoomScale**
    // zoomScale 是额外缩放， totalScale = initialFitScale * zoomScale
    // 所以 new_zoom_scale = clampedTotalScale / initialFitScale
    zoomScale.value = clampedTotalScale / initialFitScale.value;

    // Calculate new translation to keep the initial pinch midpoint fixed relative to the container center
    // Let (px, py) be the initial pinch midpoint relative to container top-left
    const px = initialPinchMidpoint.x;
    const py = initialPinchMidpoint.y;

    // Calculate the pinch midpoint relative to the container center
    const pinchMidpointRelContainerCenterX = px - containerDimensions.width / 2;
    const pinchMidpointRelContainerCenterY =
      py - containerDimensions.height / 2;

    // Calculate new translate based on the formula derived:
    // newTranslate = (pinchMidpoint - containerCenter) * (1 - actualScaleRatio) + initialTranslate * actualScaleRatio
    const newTranslateX =
      pinchMidpointRelContainerCenterX * (1 - actualScaleRatio) +
      initialImageTranslate.x * actualScaleRatio;

    const newTranslateY =
      pinchMidpointRelContainerCenterY * (1 - actualScaleRatio) +
      initialImageTranslate.y * actualScaleRatio;

    imageTranslateX.value = newTranslateX; // Update before clamping
    imageTranslateY.value = newTranslateY; // Update before clamping

    // Clamp the resulting translation immediately based on the NEW maxTranslate
    // maxTranslate computed property will react to zoomScale change, but clamping here ensures bounds
    clampCurrentTranslate();
  }
  // --- Drag Logic (Lower Priority, only if NOT pinching) ---
  else if (!isPinching.value && (isPotentialDrag.value || isDragging.value)) {
    // Ensure it's the correct input type for drag
    if (event.type.startsWith("touch") && event.touches.length !== 1) {
      // If multiple touches appear during a 1-finger drag, cancel drag state
      // The handlePointerDown for 2 fingers should handle transition to pinch
      isPotentialDrag.value = false;
      isDragging.value = false;
      return; // Stop processing drag
    }

    // Use appropriate coordinates based on event type
    const currentX = event.type.startsWith("touch")
      ? event.touches[0].clientX
      : event.clientX;
    const currentY = event.type.startsWith("touch")
      ? event.touches[0].clientY
      : event.clientY;

    const deltaX = currentX - dragStartX.value;
    const deltaY = currentY - dragStartY.value;

    // If in potential drag state, check threshold
    if (isPotentialDrag.value) {
      const moveDistance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);
      if (moveDistance > dragThreshold) {
        isDragging.value = true;
        isPotentialDrag.value = false;
        // preventDefault already handled above for touchmove/mousemove if dragging
      } else {
        return; // Not dragging yet
      }
    }

    // If in dragging state
    if (isDragging.value) {
      let newTranslateX = startTranslateX.value + deltaX;
      let newTranslateY = startTranslateY.value + deltaY;

      imageTranslateX.value = newTranslateX; // Update before clamping
      imageTranslateY.value = newTranslateY; // Update before clamping

      clampCurrentTranslate(); // Clamp immediately after setting
      // preventDefault already handled above for touchmove/mousemove if dragging
    }
  }
};

// 处理全局的 pointerup/mouseup/touchend/touchcancel 事件
const handlePointerUp = (event) => {
  // Always remove global listeners
  window.removeEventListener("mousemove", handlePointerMove);
  window.removeEventListener("mouseup", handlePointerUp);
  window.removeEventListener("touchmove", handlePointerMove);
  window.removeEventListener("touchend", handlePointerUp);
  window.removeEventListener("touchcancel", handlePointerUp);

  // Check the state *before* resetting
  const wasPinching = isPinching.value;
  const wasPotentialDrag = isPotentialDrag.value;
  const wasDragging = isDragging.value;

  // Reset drag and pinch states
  isPinching.value = false;
  isPotentialDrag.value = false;
  isDragging.value = false;

  // --- Handle transition from Pinch to Drag ---
  // If we just stopped pinching (lifted one finger) AND there is still one finger down
  if (
    wasPinching &&
    event.type.startsWith("touch") &&
    event.touches.length === 1
  ) {
    // The remaining finger starts a new potential drag gesture
    isPotentialDrag.value = true; // Enter potential drag state
    dragStartX.value = event.touches[0].clientX;
    dragStartY.value = event.touches[0].clientY;
    startTranslateX.value = imageTranslateX.value; // Start drag from current translated position
    startTranslateY.value = imageTranslateY.value;

    // Re-add touch move/end/cancel listeners for the new potential drag gesture
    // These listeners were removed at the start of handlePointerUp, so we add them back.
    // This seems a bit redundant with adding them in handlePointerDown, but ensures they are active if the gesture transitions.
    // A more sophisticated approach would track active pointers and only remove listeners when the last one is lifted.
    // For now, re-adding here after pinch ends with 1 finger is a functional approach.
    // Also, need to ensure preventDefault is called on subsequent touchmove events if a drag starts.
    // The handlePointerMove function already handles this check.
    window.addEventListener("touchmove", handlePointerMove, { passive: false });
    window.addEventListener("touchend", handlePointerUp);
    window.addEventListener("touchcancel", handlePointerUp);

    // Do NOT proceed to double tap logic below
    return;
  }

  // If it was a potential drag and did NOT become a full drag, allow handleClick to process for tap/double-tap
  // If it *was* a full drag, browser's preventDefault should stop the click event from firing anyway.
  // The handleClick function itself checks isPotentialDrag and isDragging to manage tap detection.
  // No need to manually trigger handleClick or modify lastTapTime here.
};

// --- Lifecycle Hooks ---
onMounted(() => {
  if (imageRef.value) {
    // We no longer need this listener for the problematic 360 reset logic.
    // Keeping it commented out in case it was used for other purposes.
    // imageRef.value.addEventListener("transitionend", handleTransitionEnd);
  } else {
    console.warn("Image ref is not available on mount to add event listeners.");
  }

  // Add initial pointerdown listeners to the container
  if (containerRef.value) {
    containerRef.value.addEventListener("mousedown", handlePointerDown);
    containerRef.value.addEventListener("touchstart", handlePointerDown);
    // Note: handleClick is handled via the @click directive in the template
  } else {
    console.warn(
      "Container ref is not available on mount to add pointerdown listeners."
    );
  }

  window.addEventListener("resize", handleWindowResize);

  // Ensure dimensions are updated on mount
  updateContainerDimensions();
  // If image is already complete, simulate load event
  if (imageRef.value && imageRef.value.complete) {
    onImageload();
  }
});

onUnmounted(() => {
  // Clean up states
  rotationAngle.value = 0; // Resetting angle on unmount is fine
  resetZoom(); // Also resets pinch/drag states
  lastTapTime.value = 0; // Clear timer

  // Remove event listeners
  const imageElement = imageRef.value;
  if (imageElement) {
    // Remove the transitionend listener if it was added
    // imageElement.removeEventListener("transitionend", handleTransitionEnd);
  }

  const containerElement = containerRef.value;
  if (containerElement) {
    containerElement.removeEventListener("mousedown", handlePointerDown);
    containerElement.removeEventListener("touchstart", handlePointerDown);
  }

  // Ensure removal of all possible global event listeners added during pointerdown
  window.removeEventListener("mousemove", handlePointerMove);
  window.removeEventListener("mouseup", handlePointerUp);
  window.removeEventListener("touchmove", handlePointerMove);
  window.removeEventListener("touchend", handlePointerUp);
  window.removeEventListener("touchcancel", handlePointerUp);

  // Reset transition style if it was set to 'none' - this is now handled by inline style binding based on state
  // if (imageElement && imageElement.style.transition === "none") {
  //  imageElement.style.transition = "transform 0.2s ease-in-out";
  // }

  window.removeEventListener("resize", handleWindowResize);
});

// Watch effect to ensure image dimensions are updated if the image src changes
watch(
  () => props.image.url,
  () => {
    // Reset state when image changes
    rotationAngle.value = 0; // Resetting angle on image change is reasonable
    resetZoom(); // Resets all zoom/pan/pinch/drag states
    // Wait for image to load before updating dimensions and recalculating fit
    nextTick(() => {
      if (imageRef.value && imageRef.value.complete) {
        onImageload();
      }
    });
  },
  { immediate: false } // No need to run immediately on mount as onMounted handles initial load
);
</script>

<template>
  <div
    class="image-player-container"
    ref="containerRef"
    :class="{
      'swiper-no-swiping':
        isZoomed || isDragging || isPotentialDrag || isPinching,
    }"
    @click="handleClick"
    :style="{
      // 拖拽或双指缩放状态下显示 grab/grabbing 光标
      cursor: isDragging
        ? 'grabbing'
        : isZoomed || isPotentialDrag || isPinching // 在放大或潜在拖拽/双指缩放状态显示 grab
        ? 'grab'
        : 'default',
    }"
  >
    <img
      ref="imageRef"
      :src="`${props.image.url}?auth=${token}`"
      @load="onImageload"
      :style="{
        transform: `translate(${imageTranslateX}px, ${imageTranslateY}px) scale(${currentAppliedScale}) rotate(${rotationAngle}deg)`,
        // 当处于潜在拖拽、实际拖拽或双指缩放时，暂时关闭 transform 的 transition，让手势更流畅
        transition:
          isDragging || isPotentialDrag || isPinching
            ? 'none'
            : 'transform 0.2s ease-in-out',
        'user-select': 'none', // 阻止图片被选中
        // 控制触摸事件的默认行为
        'touch-action':
          isZoomed || isPotentialDrag || isDragging || isPinching
            ? 'none'
            : 'auto', // 在放大或任何手势状态下禁用浏览器默认行为
        'pointer-events': 'auto', // Ensure image receives pointer events
      }"
      @error="handleImageError"
      draggable="false"
    />
    <div
      class="swiper-no-swiping tools"
      :class="{ active: touched }"
      @touchstart.stop="onTouchStart"
      @touchend.stop="onTouchEnd"
      @click.stop=""
      @dblclick.stop=""
    >
      <img
        class="tool-button"
        style="transform: scaleX(-1)"
        :src="RefreshIcon"
        @click.stop="rotateLeft"
      />
      <img class="tool-button" :src="RefreshIcon" @click.stop="rotateRight" />
      <Icon
        :icon="
          isAutoplayEnabled ? 'fluent:pause-28-filled' : 'fluent:play-28-filled'
        "
        class="tool-button"
        @click.stop="toggleAutoplay"
      />
    </div>
  </div>
</template>

<style scoped lang="less">
.image-player-container {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: #000;
  overflow: hidden; /* 确保图片超出容器时被裁剪 */
  // Add touch-action: none here if you want to disable *any* default touch scrolling/zooming on the container itself,
  // but be careful as it might interfere with things outside this component.
  // Relying on the image's touch-action and swiper-no-swiping is often sufficient.
}

.image-player-container img {
  display: block;
  /* transition: transform 0.2s ease-in-out; // 拖拽/缩放时在 style 绑定中动态控制 */
  max-width: none; /* 允许图片在缩放时超出原始容器尺寸 */
  max-height: none; /* 允许图片在缩放时超出原始容器尺寸 */
  // These styles help prevent image shifting or default browser behavior during gestures
  pointer-events: auto; // Ensure pointer events are captured by the image
  // touch-action: none; // Disable default touch actions like zoom/pan on the image itself (handled in inline style binding)
}

/* 工具按钮区域 */
.tools {
  position: absolute;
  right: 10px;
  bottom: 0%;
  transform: translateY(-40%);
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
  opacity: 0;
  transition: opacity 0.4s ease-in-out;
  .tool-button {
    width: 36px;
    height: 36px;
    cursor: pointer;
    flex-shrink: 0;
  }
}
.tools.active {
  opacity: 1;
  transition: opacity 0.4s ease-in-out;
}

.swiper-no-swiping {
  // This class is used by Swiper.js to prevent swiping over interactive elements.
  // No visual styles needed here.
}
</style>
