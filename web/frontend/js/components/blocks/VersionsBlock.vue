<template>
  <div class="w-full p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <h5 class="text-base font-semibold text-stone-900 dark:text-white mb-3">
      <GIcon name="info" class="mr-1" />
      {{ trans('home.versions') }}
    </h5>

    <div class="grid grid-cols-1 gap-y-4 sm:grid-cols-2 sm:gap-y-0 sm:divide-x divide-stone-200 dark:divide-stone-700">
      <section class="sm:pr-6">
        <div class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">GameAP</div>

        <div class="mt-1 text-sm text-stone-900 dark:text-white">
          <GIcon v-if="loading" name="loading" class="text-stone-400" />
          <template v-else>
            <span class="font-mono tabular-nums">{{ panelCurrent }}</span>
            <template v-if="panel.update_available">
              <span class="text-stone-400 mx-1" aria-hidden="true">&rarr;</span>
              <a
                  class="font-mono tabular-nums text-orange-500 font-medium hover:underline"
                  :href="panel.latest_stable_url"
                  target="_blank"
              >{{ panelLatest }}</a>
            </template>
          </template>
        </div>

        <div v-if="!loading && panel.update_available" class="mt-1.5">
          <GStatusBadge color="orange" :text="trans('home.update_available')" />
        </div>
        <div v-else-if="!loading && panelIsActual" class="mt-1.5">
          <GStatusBadge color="green" :text="trans('home.up_to_date')" />
        </div>
        <div
            v-else-if="!loading && panel.is_release === false && panelLatest"
            class="mt-1 text-xs text-stone-500 dark:text-stone-400"
        >
          {{ trans('home.latest_stable') }}:
          <a
              class="font-mono tabular-nums hover:underline"
              :href="panel.latest_stable_url"
              target="_blank"
          >{{ panelLatest }}</a>
        </div>

        <div v-if="panel.latest_beta" class="mt-1 text-xs text-stone-500 dark:text-stone-400">
          {{ trans('home.latest_beta') }}:
          <a
              class="font-mono tabular-nums hover:underline"
              :href="panel.latest_beta_url"
              target="_blank"
          >{{ displayVersion(panel.latest_beta) }}</a>
        </div>
      </section>

      <section class="sm:pl-6">
        <div class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">GameAP Daemon</div>

        <div class="mt-1 text-sm text-stone-900 dark:text-white">
          <GIcon v-if="loading" name="loading" class="text-stone-400" />
          <template v-else-if="daemonLatest">
            <a
                class="font-mono tabular-nums hover:underline"
                :href="daemon.latest_stable_url"
                target="_blank"
            >{{ daemonLatest }}</a>
            <span class="text-xs text-stone-500 dark:text-stone-400 ml-1.5">{{ trans('home.latest_stable') }}</span>
          </template>
          <span v-else class="text-stone-400" aria-hidden="true">&mdash;</span>
        </div>

        <div v-if="nodesTotal" class="mt-1 text-sm text-stone-500 dark:text-stone-400">
          {{ trans('home.daemons_summary', {count: nodesTotal}) }}<template v-if="offlineNodesCount"> · {{ trans('home.daemons_unavailable', {count: offlineNodesCount}) }}</template>
        </div>

        <div v-if="outdatedNodes.length" class="mt-1.5">
          <GStatusBadge
              color="orange"
              :text="trans('home.daemons_outdated', {count: outdatedNodes.length, total: nodesTotal})"
          />
        </div>
        <!-- Offline daemons have unknown versions, so "all up to date" is
             claimed only when every node was actually reachable. -->
        <div v-else-if="daemonLatest && nodesTotal && !offlineNodesCount" class="mt-1.5">
          <GStatusBadge color="green" :text="trans('home.daemons_actual')" />
        </div>

        <div v-if="visibleOutdatedNodes.length" class="mt-2">
          <router-link
              v-for="node in visibleOutdatedNodes"
              :key="node.id"
              class="flex items-center gap-x-3 py-0.5 px-1 -mx-1 rounded text-sm hover:bg-stone-100 dark:hover:bg-stone-700"
              :to="{name: 'admin.nodes.index', query: {node: String(node.id)}}"
          >
            <span class="text-stone-900 dark:text-white">{{ node.name }}</span>
            <span class="font-mono tabular-nums text-stone-500 dark:text-stone-400">{{ displayVersion(node.version) }}</span>
          </router-link>
          <router-link
              v-if="hiddenOutdatedCount"
              class="flex items-center py-0.5 px-1 -mx-1 rounded text-sm text-stone-500 dark:text-stone-400 hover:bg-stone-100 dark:hover:bg-stone-700"
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
import {GIcon, GStatusBadge} from "@gameap/ui"
import {useNodeListStore} from "@/store/nodeList"
import {useVersionStore} from "@/store/version"
import {displayVersion} from "@/utils/version"
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

const panelCurrent = computed(() => displayVersion(panel.value.current) || '—')
const panelLatest = computed(() => displayVersion(panel.value.latest_stable))
const daemonLatest = computed(() => displayVersion(daemon.value.latest_stable))

// A dev build is neither actual nor outdated, so the green badge is shown only
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
