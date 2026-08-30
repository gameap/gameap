<template>
  <GModal
      :show="show"
      :title="trans('home.daemon_versions')"
      style="width: min(96vw, 520px)"
      @update:show="(value) => emit('update:show', value)"
  >
    <div class="divide-y divide-stone-100 dark:divide-stone-700">
      <router-link
          v-for="node in nodes"
          :key="node.id"
          class="flex items-center gap-x-3 py-2 text-sm hover:bg-stone-50 dark:hover:bg-stone-700"
          :to="{name: 'admin.nodes.index', query: {node: String(node.id)}}"
          @click="emit('update:show', false)"
      >
        <span class="flex-1 truncate text-stone-900 dark:text-white">{{ node.name }}</span>
        <span class="text-stone-900 dark:text-white">{{ displayVersion(node.version) || '—' }}</span>
        <GStatusBadge :color="STATE_COLORS[nodeState(node)]" :text="trans(STATE_TEXTS[nodeState(node)])" />
      </router-link>
    </div>
  </GModal>
</template>

<script setup>
import {computed} from "vue"
import {GModal, GStatusBadge} from "@gameap/ui"
import {useNodeListStore} from "@/store/nodeList"
import {useVersionStore} from "@/store/version"
import {displayVersion} from "@/utils/version"
import {trans} from "@/i18n/i18n"

defineProps({
  show: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:show'])

const STATE_COLORS = {outdated: 'orange', actual: 'green', unknown: 'stone'}
const STATE_TEXTS = {
  outdated: 'home.version_outdated',
  actual: 'home.version_actual',
  unknown: 'home.version_unknown',
}
const STATE_ORDER = {outdated: 0, unknown: 1, actual: 2}

const versionStore = useVersionStore()
const nodeListStore = useNodeListStore()

const daemonLatestKnown = computed(() => !!versionStore.daemon.latest_stable)

// Without a known latest release a probed version cannot be called actual.
const nodeState = (node) => {
  if (node.outdated) {
    return 'outdated'
  }
  if (node.version && daemonLatestKnown.value) {
    return 'actual'
  }

  return 'unknown'
}

const nodes = computed(() => {
  const summary = nodeListStore.summary || {}
  const all = [...(summary.onlineNodes || []), ...(summary.offlineNodes || [])]

  return [...all].sort((a, b) => STATE_ORDER[nodeState(a)] - STATE_ORDER[nodeState(b)])
})
</script>
