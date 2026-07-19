<template>
  <img v-if="src" :src="src" :alt="plugin.name" :class="imgClass" @error="src = null" />
  <GIcon v-else :name="fallbackIcon" :class="fallbackClass" />
</template>

<script setup>
import { ref, watch } from 'vue'
import { GIcon } from '@gameap/ui'
import { usePluginStoreStore } from '@/store/pluginStore'

const props = defineProps({
  plugin: {
    type: Object,
    required: true
  },
  imgClass: {
    type: String,
    default: 'w-8 h-8 rounded'
  },
  fallbackIcon: {
    type: String,
    default: 'plugin'
  },
  fallbackClass: {
    type: String,
    default: 'text-2xl text-stone-400'
  }
})

const pluginStore = usePluginStoreStore()
const src = ref(null)

let requestSeq = 0
watch(
  () => [props.plugin?.id, props.plugin?.icon_url],
  async () => {
    const seq = ++requestSeq
    src.value = null
    const url = await pluginStore.fetchPluginIcon(props.plugin)
    if (seq === requestSeq) {
      src.value = url
    }
  },
  { immediate: true }
)
</script>
