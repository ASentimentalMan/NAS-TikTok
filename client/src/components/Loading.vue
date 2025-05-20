<script setup lang="ts">
defineExpose({ name: "Loading" });
const props = defineProps({
  isFullScreen: {
    type: Boolean,
    default: true,
  },
});
</script>

<template>
  <div class="Loading" :class="[props.isFullScreen ? 'full' : 'inline']">
    <div class="circle blue"></div>
    <div class="circle red"></div>
  </div>
</template>

<style scoped lang="less">
.Loading {
  &.inline {
    width: 100%;
    height: 40px;
    display: flex;
    justify-content: center;
    align-items: center;
  }

  &.small {
    .circle {
      width: 8px;
      height: 8px;
    }
  }

  &.full {
    z-index: 999;
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    display: flex;
    justify-content: space-between;
    width: 22px;
  }

  .circle {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }

  .blue {
    background: cadetblue;
    animation: anim-blue 0.4s ease-in-out 0s infinite alternate;
  }

  .red {
    background: rgb(252, 47, 86);
    animation: anim-red 0.4s ease-in-out 0s infinite alternate;
  }

  @keyframes anim-blue {
    from {
      transform: translate3d(0, 0, 0) scale(1);
    }
    to {
      transform: translate3d(10px, 0, 0) scale(1.2);
    }
  }
  @keyframes anim-red {
    from {
      transform: translate3d(0, 0, 0) scale(1);
    }
    to {
      transform: translate3d(-10px, 0, 0) scale(1.2);
    }
  }
}
</style>
