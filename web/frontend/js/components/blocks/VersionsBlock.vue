<template>
  <div class="w-full p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <div class="md:grid md:grid-cols-5 md:gap-4">
      <h5 class="col-span-3 text-base inline-block align-middle font-semibold text-stone-900 dark:text-white max-md:mb-2">
        <GIcon name="info" class="mr-1" />
        {{ trans('home.versions') }}
      </h5>

      <div class="col-span-2 text-sm">
        <div class="flex items-center flex-wrap gap-x-2 py-1">
          <span class="font-medium text-stone-900 dark:text-white">GameAP</span>
          <GIcon v-if="loading" name="loading" class="animate-spin" />
          <template v-else>
            <span class="text-stone-600 dark:text-stone-300">{{ panelCurrent }}</span>

            <template v-if="panel.update_available">
              <span class="text-stone-400" aria-hidden="true">&rarr;</span>
              <a
                  class="text-orange-500 font-medium hover:underline"
                  :href="panel.latest_stable_url"
                  target="_blank"
              >{{ panel.latest_stable }}</a>
              <NTooltip trigger="hover">
                <template #trigger>
                  <GIcon name="warning" class="text-orange-500" />
                </template>
                {{ trans('home.old_version') }}
              </NTooltip>
            </template>

            <NTooltip v-else-if="panelIsActual" trigger="hover">
              <template #trigger>
                <GIcon name="check" class="text-lime-600" />
              </template>
              {{ trans('home.actual_version') }}
            </NTooltip>

            <NTooltip v-else-if="panel.is_release === false" trigger="hover">
              <template #trigger>
                <GIcon name="question" class="text-stone-400" />
              </template>
              {{ trans('home.dev_version') }}
            </NTooltip>
          </template>
        </div>

        <div v-if="panel.latest_beta" class="py-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.latest_beta') }}:
          <a class="hover:underline" :href="panel.latest_beta_url" target="_blank">{{ panel.latest_beta }}</a>
        </div>

        <div class="flex items-center flex-wrap gap-x-2 py-1 mt-1">
          <span class="font-medium text-stone-900 dark:text-white">gameap-daemon</span>
          <span class="text-stone-600 dark:text-stone-300">
            {{ trans('home.daemons_summary', {count: nodesTotal}) }}
          </span>
          <template v-if="daemon.latest_stable">
            <span class="text-stone-400">&middot;</span>
            <span class="text-stone-600 dark:text-stone-300">
              {{ trans('home.latest_stable') }}:
              <a class="hover:underline" :href="daemon.latest_stable_url" target="_blank">{{ daemon.latest_stable }}</a>
            </span>
          </template>
        </div>

        <router-link
            v-for="node in outdatedNodes"
            :key="node.id"
            class="flex items-center gap-x-2 py-1 px-1 -mx-1 rounded hover:bg-stone-100 dark:hover:bg-stone-700"
            :to="{name: 'admin.nodes.index', query: {node: String(node.id)}}"
        >
          <GIcon name="warning" class="text-orange-500" />
          <span class="text-stone-900 dark:text-white">{{ node.name }}</span>
          <span class="text-stone-500 dark:text-stone-400">{{ node.version }}</span>
        </router-link>

        <div v-if="outdatedNodes.length" class="py-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.daemons_outdated', {count: outdatedNodes.length, total: nodesTotal}) }}
        </div>
        <div v-else-if="daemon.latest_stable && nodesTotal" class="py-1 text-xs text-lime-600">
          <GIcon name="check" class="mr-1" />
          {{ trans('home.daemons_actual') }}
        </div>

        <div v-if="offlineNodesCount" class="py-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.daemons_unavailable', {count: offlineNodesCount}) }}
        </div>

        <div v-if="!updateCheckEnabled && !loading" class="py-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.update_check_disabled') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import {computed, onMounted} from "vue"
import {NTooltip} from "naive-ui"
import {GIcon} from "@gameap/ui"
import {useNodeListStore} from "@/store/nodeList"
import {useVersionStore} from "@/store/version"
import {trans} from "@/i18n/i18n"
import {errorNotification} from "@/parts/dialogs"

const versionStore = useVersionStore()
const nodeListStore = useNodeListStore()

const loading = computed(() => versionStore.loading)
const panel = computed(() => versionStore.panel)
const daemon = computed(() => versionStore.daemon)
const updateCheckEnabled = computed(() => versionStore.updateCheckEnabled)

const panelCurrent = computed(() => panel.value.current || '—')

// A dev build is neither actual nor outdated, so the check mark is shown only
// for a release build the update check managed to compare.
const panelIsActual = computed(() => {
  return panel.value.is_release === true && !!panel.value.latest_stable
})

const onlineNodes = computed(() => nodeListStore.summary?.onlineNodes || [])
const offlineNodesCount = computed(() => (nodeListStore.summary?.offlineNodes || []).length)
const nodesTotal = computed(() => nodeListStore.summary?.total || 0)
const outdatedNodes = computed(() => onlineNodes.value.filter((node) => node.outdated))

onMounted(() => {
  versionStore.fetchVersion().catch((error) => {
    errorNotification(error)
  })
})
</script>
