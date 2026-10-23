<script setup>
import { ref, watch } from "vue";
import SimpleToast from "@/components/SimpleToast.vue";
import { credentialPersistenceNotice } from "@/utils/saved-credentials";

const persistenceToastRef = ref(null);

watch(
  credentialPersistenceNotice,
  (message) => {
    if (!message) return;
    persistenceToastRef.value?.show(message);
    credentialPersistenceNotice.value = "";
  },
  { flush: "post" }
);
</script>

<template>
  <RouterView />
  <SimpleToast ref="persistenceToastRef" />
</template>
