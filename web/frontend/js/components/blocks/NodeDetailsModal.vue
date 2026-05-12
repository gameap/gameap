<template>
  <n-modal
      :show="show"
      :on-update:show="(v) => $emit('update:show', v)"
      :auto-focus="false"
      preset="card"
      style="width: min(96vw, 1300px); max-width: 1300px;"
      :bordered="false"
      :segmented="{ content: 'soft', footer: 'soft' }"
      :close-on-esc="true"
      :mask-closable="true"
  >
    <template #header>
      <div class="flex items-center gap-3 flex-wrap">
        <GIcon :name="osIcon" class="text-xl" />
        <span class="font-semibold">{{ node?.name || `node #${nodeId}` }}</span>
        <n-tag
            v-if="online"
            type="success"
            size="small"
            round
            :bordered="false"
        >
          {{ trans('dedicated_servers.online') }}
        </n-tag>
        <n-tag
            v-else
            type="error"
            size="small"
            round
            :bordered="false"
        >
          {{ trans('dedicated_servers.offline') }}
        </n-tag>
      </div>
    </template>

    <template #header-extra>
      <div class="flex flex-wrap gap-1.5 mr-10">
        <GButton
            color="blue"
            size="small"
            :route="{ name: 'admin.nodes.edit', params: { id: nodeId } }"
        >
          <GIcon name="edit" class="mr-0.5" />
          <span>{{ trans('main.edit') }}</span>
        </GButton>
        <GButton color="orange" size="small" :disabled="downloading" @click="downloadLogs">
          <GIcon name="download" class="mr-0.5" />
          <span>{{ trans('dedicated_servers.download_logs') }}</span>
        </GButton>
        <GButton color="green" size="small" :disabled="downloading" @click="downloadCertificates">
          <GIcon name="certificate" class="mr-0.5" />
          <span>{{ trans('dedicated_servers.download_certificates') }}</span>
        </GButton>
        <GButton color="red" size="small" @click="onDelete">
          <GIcon name="delete" class="mr-0.5" />
          <span>{{ trans('main.delete') }}</span>
        </GButton>
      </div>
    </template>

    <div v-if="downloading" class="mb-3">
      <Progressbar :progress="downloadProgress" />
    </div>

    <div class="overflow-y-auto pr-1 max-h-[75vh]">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="overview" :tab="trans('dedicated_servers.tab_overview')">
          <Loading v-if="!node && loading" />
          <NodeOverviewTab v-else :node="node" :daemon-info="daemonInfo" />
        </n-tab-pane>

        <n-tab-pane name="metrics" :tab="trans('dedicated_servers.tab_metrics')">
          <NodeMetricsTab v-if="show && online" :node-id="nodeId" />
          <div v-else-if="show && !online" class="text-center text-stone-500 py-12">
            {{ trans('dedicated_servers.no_metrics_data') }}
          </div>
        </n-tab-pane>
      </n-tabs>
    </div>
  </n-modal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { NModal, NTabs, NTabPane, NTag } from 'naive-ui'
import { GIcon, Loading, Progressbar } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import NodeOverviewTab from './NodeOverviewTab.vue'
import NodeMetricsTab from './NodeMetricsTab.vue'
import { useNodeStore } from '@/store/node'
import { errorNotification, notification } from '@/parts/dialogs'
import { trans } from '@/i18n/i18n'
import axios from '@/config/axios'
import { storeToRefs } from 'pinia'

const props = defineProps({
    show: { type: Boolean, default: false },
    nodeId: { type: [Number, String], default: null },
    node: { type: Object, default: null },
    online: { type: Boolean, default: false },
})

const emit = defineEmits(['update:show', 'deleted'])

const nodeStore = useNodeStore()
const { daemonInfo, loading } = storeToRefs(nodeStore)

const activeTab = ref('overview')
const downloading = ref(false)
const downloadProgress = ref(0)

const osIcon = computed(() => {
    const os = String(props.node?.os || '').toLowerCase()
    if (os.startsWith('w')) return 'windows'
    if (os.startsWith('m')) return 'apple'
    return 'linux'
})

watch(
    () => [props.show, props.nodeId],
    ([show, id]) => {
        if (show && id) {
            nodeStore.setNodeId(id)
            nodeStore.fetchDaemonInfo().catch((error) => {
                errorNotification(error)
            })
            activeTab.value = 'overview'
        }
    },
    { immediate: true },
)

async function downloadFile(url, filename) {
    downloading.value = true
    downloadProgress.value = 0

    try {
        const response = await axios.get(url, {
            responseType: 'blob',
            onDownloadProgress: (progressEvent) => {
                if (progressEvent.total) {
                    downloadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
                }
            },
        })

        const blob = new Blob([response.data])
        const link = document.createElement('a')
        link.href = window.URL.createObjectURL(blob)
        link.setAttribute('download', filename)
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
        window.URL.revokeObjectURL(link.href)
    } catch (error) {
        errorNotification(error)
    } finally {
        downloading.value = false
        downloadProgress.value = 0
    }
}

function downloadLogs() {
    downloadFile(`/api/nodes/${props.nodeId}/logs.zip`, 'logs.zip')
}

function downloadCertificates() {
    downloadFile('/api/nodes/certificates.zip', 'certificates.zip')
}

function onDelete() {
    window.$dialog.error({
        title: trans('dedicated_servers.delete_confirm_msg'),
        positiveText: trans('main.yes'),
        negativeText: trans('main.no'),
        closable: false,
        onPositiveClick: async () => {
            try {
                await axios.delete(`/api/nodes/${props.nodeId}`)
                notification({
                    content: trans('dedicated_servers.delete_success_msg'),
                    type: 'success',
                })
                emit('deleted', props.nodeId)
                emit('update:show', false)
            } catch (error) {
                errorNotification(error)
            }
        },
    })
}
</script>
