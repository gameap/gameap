<template>
  <RouterView :key="$route.path"/>
</template>

<script setup>
import { useMessage, useDialog } from "naive-ui"
import { useRouter, useRoute } from "vue-router"
import {onMounted} from "vue";

const router = useRouter()
const route = useRoute()

window.$message = useMessage();
window.$dialog = useDialog();

onMounted(() => {
  // Router already handles navigation correctly, no need to manually replace
  // The previous implementation caused duplicate component mounting on page load
});
</script>