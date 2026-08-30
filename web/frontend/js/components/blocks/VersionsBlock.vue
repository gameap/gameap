<template>
  <div class="w-full p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <div class="grid grid-cols-1 gap-y-6 sm:grid-cols-2 sm:gap-y-0 sm:divide-x divide-stone-200 dark:divide-stone-700">
      <section class="sm:pr-6">
        <div class="text-base font-semibold text-stone-900 dark:text-white">GameAP</div>

        <GIcon v-if="versionLoading" name="loading" class="mt-3 text-stone-400" />
        <template v-else>
          <div class="mt-2 space-y-1 text-sm text-stone-900 dark:text-white">
            <div v-if="panelLatestStable">
              {{ trans('home.latest_stable') }}:
              <a class="hover:underline" :href="panel.latest_stable_url" target="_blank">{{ panelLatestStable }}</a>
            </div>
            <div v-if="panelLatestBeta">
              {{ trans('home.latest_beta') }}:
              <a class="hover:underline" :href="panel.latest_beta_url" target="_blank">{{ panelLatestBeta }}</a>
            </div>
          </div>

          <div class="mt-3 text-sm font-semibold text-stone-900 dark:text-white">{{ trans('home.version_in_use') }}</div>
          <div class="mt-1.5">
            <GStatusBadge :color="panelBadgeColor" :text="panelCurrent" />
          </div>
        </template>
      </section>

      <section class="sm:pl-6">
        <div class="text-base font-semibold text-stone-900 dark:text-white">GameAP Daemon</div>

        <GIcon v-if="versionLoading || summaryLoading" name="loading" class="mt-3 text-stone-400" />
        <template v-else>
          <div class="mt-2 space-y-1 text-sm text-stone-900 dark:text-white">
            <div v-if="daemonLatestStable">
              {{ trans('home.latest_stable') }}:
              <a class="hover:underline" :href="daemon.latest_stable_url" target="_blank">{{ daemonLatestStable }}</a>
            </div>
            <div v-if="daemonLatestBeta">
              {{ trans('home.latest_beta') }}:
              <a class="hover:underline" :href="daemon.latest_beta_url" target="_blank">{{ daemonLatestBeta }}</a>
            </div>
          </div>

          <template v-if="daemonStatus">
            <div class="mt-3 text-sm font-semibold text-stone-900 dark:text-white">{{ trans('home.version_in_use') }}</div>
            <div class="mt-1.5">
              <button
                  v-if="daemonStatusClickable"
                  type="button"
                  class="cursor-pointer hover:opacity-80"
                  @click="nodesModalShown = true"
              >
                <GStatusBadge :color="daemonStatusColor" :text="daemonStatusText" />
              </button>
              <GStatusBadge v-else :color="daemonStatusColor" :text="daemonStatusText" />
            </div>
          </template>
        </template>
      </section>
    </div>

    <div v-if="fetched && !updateCheckEnabled" class="mt-3 text-sm text-stone-500 dark:text-stone-400">
      {{ trans('home.update_check_disabled') }}
    </div>

    <DaemonVersionsModal v-model:show="nodesModalShown" />
  </div>
</template>

<script setup>
import {computed, onMounted, ref} from "vue"
import {GIcon, GStatusBadge} from "@gameap/ui"
import DaemonVersionsModal from "@/components/blocks/DaemonVersionsModal.vue"
import {useNodeListStore} from "@/store/nodeList"
import {useVersionStore} from "@/store/version"
import {displayVersion} from "@/utils/version"
import {trans} from "@/i18n/i18n"
import {errorNotification} from "@/parts/dialogs"

const versionStore = useVersionStore()
const nodeListStore = useNodeListStore()

const nodesModalShown = ref(false)

const versionLoading = computed(() => versionStore.loading)
const summaryLoading = computed(() => nodeListStore.loading)
// A failed or still-running request must not read as "update check is
// disabled" — the note is shown only for a successfully loaded response.
const fetched = computed(() => versionStore.fetched)
const panel = computed(() => versionStore.panel)
const daemon = computed(() => versionStore.daemon)
const updateCheckEnabled = computed(() => versionStore.updateCheckEnabled)

const panelCurrent = computed(() => displayVersion(panel.value.current) || '—')
const panelLatestStable = computed(() => displayVersion(panel.value.latest_stable))
const panelLatestBeta = computed(() => displayVersion(panel.value.latest_beta))
const daemonLatestStable = computed(() => displayVersion(daemon.value.latest_stable))
const daemonLatestBeta = computed(() => displayVersion(daemon.value.latest_beta))

// A dev build is neither actual nor outdated, so the green badge is shown only
// for a release build the update check managed to compare.
const panelBadgeColor = computed(() => {
  if (panel.value.update_available) {
    return 'orange'
  }
  if (panel.value.is_release === true && panel.value.latest_stable) {
    return 'green'
  }

  return 'stone'
})

const summary = computed(() => nodeListStore.summary || {})
const summaryLoaded = computed(() => typeof summary.value.total === 'number')
const allNodes = computed(() => [
  ...(summary.value.onlineNodes || []),
  ...(summary.value.offlineNodes || []),
])
const nodesTotal = computed(() => summary.value.total || 0)
const outdatedCount = computed(() => allNodes.value.filter((node) => node.outdated).length)
const knownVersionCount = computed(() => allNodes.value.filter((node) => node.version).length)

// Offline daemons have unknown versions, so "all up to date" is claimed only
// when every daemon version was actually compared against a known release.
const daemonStatus = computed(() => {
  if (!updateCheckEnabled.value) {
    return ''
  }
  if (!summaryLoaded.value) {
    return 'unavailable'
  }
  if (nodesTotal.value === 0) {
    return ''
  }
  if (outdatedCount.value > 0) {
    return 'outdated'
  }
  if (daemon.value.latest_stable && knownVersionCount.value === nodesTotal.value) {
    return 'actual'
  }

  return 'unavailable'
})

const daemonStatusClickable = computed(() => ['outdated', 'actual'].includes(daemonStatus.value))

const daemonStatusColor = computed(() => {
  return {outdated: 'orange', actual: 'green', unavailable: 'red'}[daemonStatus.value]
})

const daemonStatusText = computed(() => {
  switch (daemonStatus.value) {
  case 'outdated':
    return trans('home.daemons_outdated', {count: outdatedCount.value, total: nodesTotal.value})
  case 'actual':
    return trans('home.daemons_actual')
  default:
    return trans('home.version_info_unavailable')
  }
})

onMounted(() => {
  versionStore.ensureFetched().catch((error) => {
    errorNotification(error)
  })
})
</script>
