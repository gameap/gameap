<template>
  <div class="w-full p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <h5 class="text-base font-semibold text-stone-900 dark:text-white mb-3">
      <GIcon name="info" class="mr-1" />
      {{ trans('home.versions') }}
    </h5>

    <div class="grid grid-cols-1 gap-y-4 sm:grid-cols-2 sm:gap-y-0 sm:divide-x divide-stone-200 dark:divide-stone-700">
      <section class="sm:pr-6">
        <div class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">GameAP</div>

        <div class="mt-1 flex items-center flex-wrap gap-x-2">
          <GIcon v-if="loading" name="loading" class="text-stone-400" />
          <template v-else>
            <span class="font-mono tabular-nums text-lg font-semibold text-stone-900 dark:text-white">{{ panelCurrent }}</span>

            <template v-if="panel.update_available">
              <span class="text-stone-400" aria-hidden="true">&rarr;</span>
              <a
                  class="font-mono tabular-nums text-lg font-semibold text-orange-500 hover:underline"
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

        <div v-if="!loading && (panel.update_available || panelIsActual)" class="mt-1 text-xs">
          <span v-if="panel.update_available" class="text-orange-500">{{ trans('home.update_available') }}</span>
          <span v-else class="text-lime-600">{{ trans('home.up_to_date') }}</span>
        </div>

        <div v-if="panel.latest_beta" class="mt-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.latest_beta') }}:
          <a class="hover:underline" :href="panel.latest_beta_url" target="_blank">{{ panel.latest_beta }}</a>
        </div>
      </section>

      <section class="sm:pl-6">
        <div class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">GameAP Daemon</div>

        <div class="mt-1 flex items-baseline flex-wrap gap-x-2">
          <GIcon v-if="loading" name="loading" class="text-stone-400" />
          <template v-else-if="daemon.latest_stable">
            <a
                class="font-mono tabular-nums text-lg font-semibold text-stone-900 dark:text-white hover:underline"
                :href="daemon.latest_stable_url"
                target="_blank"
            >{{ daemon.latest_stable }}</a>
            <span class="text-xs text-stone-500 dark:text-stone-400">{{ trans('home.latest_stable') }}</span>
          </template>
          <span v-else class="font-mono text-lg font-semibold text-stone-400" aria-hidden="true">&mdash;</span>
        </div>

        <div v-if="nodesTotal" class="mt-1 text-xs flex items-center flex-wrap gap-x-2 gap-y-0.5">
          <span class="text-stone-500 dark:text-stone-400">{{ trans('home.daemons_summary', {count: nodesTotal}) }}</span>
          <span v-if="outdatedNodes.length" class="text-orange-500">
            {{ trans('home.daemons_outdated', {count: outdatedNodes.length, total: nodesTotal}) }}
          </span>
          <span v-else-if="daemon.latest_stable" class="text-lime-600">
            <GIcon name="check" class="mr-0.5" />{{ trans('home.daemons_actual') }}
          </span>
          <span v-if="offlineNodesCount" class="text-stone-500 dark:text-stone-400">
            {{ trans('home.daemons_unavailable', {count: offlineNodesCount}) }}
          </span>
        </div>

        <div v-if="visibleOutdatedNodes.length" class="mt-2 flex flex-wrap gap-1.5">
          <router-link
              v-for="node in visibleOutdatedNodes"
              :key="node.id"
              class="inline-flex items-center gap-x-1 px-2 py-0.5 rounded text-xs bg-warning-soft text-warning-soft-text hover:opacity-80"
              :to="{name: 'admin.nodes.index', query: {node: String(node.id)}}"
          >
            <GIcon name="warning" />
            <span class="font-medium">{{ node.name }}</span>
            <span class="font-mono tabular-nums">{{ node.version }}</span>
          </router-link>
          <router-link
              v-if="hiddenOutdatedCount"
              class="inline-flex items-center px-2 py-0.5 rounded text-xs bg-stone-200 text-stone-600 hover:opacity-80 dark:bg-stone-700 dark:text-stone-300"
              :to="{name: 'admin.nodes.index'}"
          >+{{ hiddenOutdatedCount }}</router-link>
        </div>
      </section>
    </div>

    <div v-if="fetched && !updateCheckEnabled" class="mt-3 text-xs text-stone-500 dark:text-stone-400">
      {{ trans('home.update_check_disabled') }}
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

const OUTDATED_NODES_LIMIT = 8

const versionStore = useVersionStore()
const nodeListStore = useNodeListStore()

const loading = computed(() => versionStore.loading)
// A failed or still-running request must not read as "update check is
// disabled" — the note is shown only for a successfully loaded response.
const fetched = computed(() => versionStore.fetched)
const panel = computed(() => versionStore.panel)
const daemon = computed(() => versionStore.daemon)
const updateCheckEnabled = computed(() => versionStore.updateCheckEnabled)

const panelCurrent = computed(() => panel.value.current || '—')

// A dev build is neither actual nor outdated, so the check mark is shown only
// for a release build the update check managed to compare.
const panelIsActual = computed(() => {
  return panel.value.is_release === true && !!panel.value.latest_stable && !panel.value.update_available
})

const onlineNodes = computed(() => nodeListStore.summary?.onlineNodes || [])
const offlineNodesCount = computed(() => (nodeListStore.summary?.offlineNodes || []).length)
const nodesTotal = computed(() => nodeListStore.summary?.total || 0)
const outdatedNodes = computed(() => onlineNodes.value.filter((node) => node.outdated))
const visibleOutdatedNodes = computed(() => outdatedNodes.value.slice(0, OUTDATED_NODES_LIMIT))
const hiddenOutdatedCount = computed(() => Math.max(0, outdatedNodes.value.length - OUTDATED_NODES_LIMIT))

onMounted(() => {
  versionStore.ensureFetched().catch((error) => {
    errorNotification(error)
  })
})
</script>
