<script setup>
// 定义组件接收的 props
const props = defineProps({
  tabs: {
    type: Array,
    required: true,
  },
  activeTabIndex: {
    type: Number,
    required: true,
  },
  underlineStyle: {
    type: Object,
    required: true,
  },
});
const standalone = inject("standalone");

// 定义组件可以触发的事件
const emit = defineEmits(["tab-click"]);

// 标签容器的引用
const tabsContainerRef = ref(null);
// 各个标签项的引用数组
const tabItemRefs = ref([]);

// 当标签项被点击时，触发 tab-click 事件，并将索引传递给父组件
const emitTabClick = (index) => {
  emit("tab-click", index);
};

// 向父组件暴露 tabItemRefs，以便父组件可以获取标签元素的 DOM 引用
const getTabItemRefs = () => tabItemRefs.value;
defineExpose({
  getTabItemRefs,
  // 也可以选择暴露 tabsContainerRef 如果父组件需要访问容器
  // tabsContainerRef
});

// 注意：这里的 underlineStyle prop 是一个 reactive 对象，
// 父组件直接修改这个对象的属性就会反映到这里，不需要 watch。
// 如果需要监听 tabs 或 activeTabIndex 的变化来执行本地逻辑，可以使用 watch。

// 例如，可以在 tabs 或 activeTabIndex 变化时执行一些本地清理或设置
// watch(() => props.activeTabIndex, (newIndex, oldIndex) => {
//   console.log(`Tab changed from ${oldIndex} to ${newIndex}`);
//   // 可以在这里执行一些与标签状态变化相关的本地操作
// });
</script>

<template>
  <div class="tabs-container" ref="tabsContainerRef">
    <div
      v-for="(tab, index) in tabs"
      :key="index"
      class="tab-item"
      :class="{ active: activeTabIndex === index, standalone }"
      @click="emitTabClick(index)"
      ref="tabItemRefs"
    >
      {{ tab.label }}
    </div>
    <div class="tab-underline" :style="underlineStyle">
      <div class="indicator"></div>
    </div>
  </div>
</template>

<style scoped lang="less">
/* 标签容器样式 */
.tabs-container {
  display: flex;
  justify-content: center;
  position: fixed; /* 为下划线的绝对定位提供参考 */
  left: 0;
  top: env(safe-area-inset-top, 0);
  width: 100vw;
  z-index: 10; /* 确保标签层级高于 Swiper 内容 */
}

/* 单个标签项样式 */
.tab-item {
  padding: 8px 12px; /* 增加内边距，改善点击区域 */
  cursor: pointer; /* 鼠标悬停时显示手型 */
  text-align: center;
  /*flex-grow: 1;  让标签项平均分配可用空间 */
  color: rgba(255, 255, 255, 0.7); /* 默认字体颜色 */
  font-weight: 500;
  transition: color 0.3s ease; /* 字体颜色过渡 */
}
& .standalone {
  padding: 0 12px 5px 12px; /* 增加内边距，改善点击区域 */
}
/* 激活状态的标签项样式 */
.tab-item.active {
  color: white; /* 激活状态的字体颜色 */
  font-weight: 700; /* 激活状态的字体加粗 */
}

/* 下划线样式 (位置和宽度由父组件通过 style 绑定控制) */
.tab-underline {
  position: absolute;
  bottom: 0;
  display: inline-flex;
  justify-content: center;
}

.indicator {
  height: 2px; /* 下划线高度 */
  width: 20px;
  background: #fff; /* 下划线颜色 */
  /* 过渡效果在父组件中根据不同情况动态设置 */
  border-radius: 5rem;
}
</style>
