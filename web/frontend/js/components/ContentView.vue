<template>
  <PluginSlot
      v-if="pluginsStore.isInitialized"
      name="global-banners"
      :context="{ routeName: route.name, isAdmin }"
  />

  <PluginSlot
      v-if="pluginsStore.isInitialized && isAdminSection"
      name="admin-pages"
      :context="{ routeName: route.name, isAdmin }"
  />

  <RouterView :key="$route.path"/>
</template>

<script setup>
import { useMessage, useDialog } from "naive-ui"
import { useRouter, useRoute } from "vue-router"
import {computed, onMounted} from "vue";
import {useAuthStore} from "@/store/auth";
import {usePluginsStore} from "@/store/plugins";
import PluginSlot from "@/plugins/components/PluginSlot.vue";

const router = useRouter()
const route = useRoute()

const authStore = useAuthStore()
const pluginsStore = usePluginsStore()

const isAdmin = computed(() => authStore.isAdmin)

// `admin-pages` is scoped to the administrative section, unlike `global-banners`
// which every page renders for every user.
const isAdminSection = computed(() => isAdmin.value && route.path.startsWith('/admin'))

window.$message = useMessage();
window.$dialog = useDialog();

onMounted(() => {
  // Router already handles navigation correctly, no need to manually replace
  // The previous implementation caused duplicate component mounting on page load
});
</script>
