<script setup>
const visible = ref(false);
const message = ref("");

const show = (msg, duration = 2000) => {
  message.value = msg;
  visible.value = true;
  setTimeout(() => {
    visible.value = false;
  }, duration);
};

defineExpose({ show });
</script>

<template>
  <transition name="fade">
    <div v-if="visible" class="toast">{{ message }}</div>
  </transition>
</template>

<style scoped>
.toast {
  position: fixed;
  bottom: 20%;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(0, 0, 0, 0.75);
  color: white;
  padding: 10px 16px;
  border-radius: 6px;
  font-size: 14px;
  z-index: 9999;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
