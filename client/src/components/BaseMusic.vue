<script setup lang="ts">
import { Icon } from "@iconify/vue";
import MusicIcon from "@/assets/imgs/music.png";
const props = defineProps({
  isPlaying: {
    type: Boolean,
    required: false,
  },
  muted: {
    type: Boolean,
    required: true,
  },
  index: {
    type: Number,
    default: 0,
  },
});

let showMutedNotice = ref(false);

const style = computed(() => {
  return { webkitAnimationPlayState: props.isPlaying ? "running" : "paused" };
});

watch(
  () => props.muted,
  (newValue) => {
    if (newValue && props.index === 0) {
      showMutedNotice.value = true;
      setTimeout(() => {
        showMutedNotice.value = false;
      }, 1000);
    }
  },
  { immediate: true }
);
</script>

<template>
  <div class="music-wrapper">
    <div
      class="mute-icon"
      :class="showMutedNotice && 'notice'"
      v-if="props.muted"
    >
      <div class="wrap">
        <Icon icon="flowbite:volume-mute-solid" />
        <span :style="{ opacity: showMutedNotice ? 1 : 0 }">取消静音</span>
      </div>
    </div>
    <img class="music" :src="MusicIcon" :style="style" />
  </div>
</template>

<style scoped lang="less">
.music-wrapper {
  display: flex;
  justify-content: center;
  @w: 45px;
  width: @w;
  height: @w;
  position: relative;

  .music {
    border-radius: 50%;
    width: 100%;
    height: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
    animation: animations 5s linear forwards infinite;
    //animation-play-state:paused;
    //display: none;
  }

  @keyframes animations {
    0% {
      transform: rotate(0deg);
    }
    100% {
      transform: rotate(360deg);
    }
  }

  .mute-icon {
    .music;
    cursor: pointer;
    position: absolute;
    z-index: 1;
    right: 0;
    background: white;
    animation: unset;
    color: black;
    transition: all 0.5s;
    overflow: hidden;

    .wrap {
      width: 100px;
      position: absolute;
      left: 0;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    &.notice {
      border-radius: 50px;
      width: 100px;
    }

    svg {
      font-size: 22px;
    }

    span {
      margin-left: 5px;
      font-size: 13px;
      word-break: keep-all;
      transition: all 0.5s;
    }
  }
}
</style>
