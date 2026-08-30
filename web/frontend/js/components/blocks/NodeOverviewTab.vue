<template>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <GCard :title="trans('dedicated_servers.title_view')">
      <GTable>
        <tbody>
          <tr>
            <td><strong>{{ trans('dedicated_servers.gdaemon_uptime') }}:</strong></td>
            <td>{{ daemonInfo?.base_info?.uptime || '—' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.gdaemon_version') }}:</strong></td>
            <td>
              <span class="inline-flex items-center flex-wrap gap-x-1.5">
                <span>{{ versionLine }}</span>
                <template v-if="outdated && latestStable">
                  <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                  <a
                      class="text-orange-500 font-medium hover:underline"
                      :href="latestStableUrl"
                      target="_blank"
                  >{{ latestStable }}</a>
                  <NTooltip trigger="hover">
                    <template #trigger>
                      <GIcon name="warning" class="text-orange-500" />
                    </template>
                    {{ trans('dedicated_servers.daemon_update_available') }}
                  </NTooltip>
                </template>
                <NTooltip v-else-if="daemonUpToDate" trigger="hover">
                  <template #trigger>
                    <GIcon name="check" class="text-lime-600" />
                  </template>
                  {{ trans('dedicated_servers.daemon_up_to_date') }}
                </NTooltip>
              </span>
            </td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.gdaemon_online_servers_count') }}:</strong></td>
            <td>{{ daemonInfo?.base_info?.online_servers_count || '0' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.gdaemon_working_tasks_count') }}:</strong></td>
            <td>{{ daemonInfo?.base_info?.working_tasks_count || '0' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.gdaemon_waiting_tasks_count') }}:</strong></td>
            <td>{{ daemonInfo?.base_info?.waiting_tasks_count || '0' }}</td>
          </tr>
        </tbody>
      </GTable>
    </GCard>

    <GCard :title="trans('main.details')">
      <GTable>
        <tbody>
          <tr>
            <td><strong>{{ trans('dedicated_servers.location') }}:</strong></td>
            <td>{{ node?.location || '—' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.provider') }}:</strong></td>
            <td>{{ node?.provider || '—' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.os') }}:</strong></td>
            <td>
              <GIcon :name="osIcon" class="mr-1" />
              {{ node?.os || '—' }}
            </td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.ip') }}:</strong></td>
            <td>{{ ipString || '—' }}</td>
          </tr>
          <tr>
            <td><strong>{{ trans('dedicated_servers.work_path') }}:</strong></td>
            <td class="break-all">{{ node?.work_path || '—' }}</td>
          </tr>
        </tbody>
      </GTable>
    </GCard>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { NTooltip } from 'naive-ui'
import { GCard, GTable, GIcon } from '@gameap/ui'
import { useVersionStore } from '@/store/version'
import { trans } from '@/i18n/i18n'

const props = defineProps({
    node: { type: Object, default: null },
    daemonInfo: { type: Object, default: null },
    outdated: { type: Boolean, default: false },
})

const versionStore = useVersionStore()

const latestStable = computed(() => versionStore.daemon.latest_stable || '')
const latestStableUrl = computed(() => versionStore.daemon.latest_stable_url || '')

// The daemon reports a bare semver and the release feed strips the "v" prefix,
// so plain equality is enough; a dev daemon build matches nothing and stays
// unmarked.
const daemonUpToDate = computed(() => {
    const current = props.daemonInfo?.version?.version

    return !!current && !!latestStable.value && !props.outdated && current === latestStable.value
})

const versionLine = computed(() => {
    const v = props.daemonInfo?.version
    if (!v?.version) return '—'
    return v.compile_date ? `${v.version} (${v.compile_date})` : v.version
})

const osIcon = computed(() => {
    const os = String(props.node?.os || '').toLowerCase()
    if (os.startsWith('w')) return 'windows'
    if (os.startsWith('m')) return 'apple'
    return 'linux'
})

const ipString = computed(() => {
    const ip = props.node?.ip
    if (!Array.isArray(ip) || ip.length === 0) return ''
    return ip.join(', ')
})
</script>
