<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <div class="mb-4 flex flex-wrap gap-2">
    <GButton color="green" size="middle" @click="showCreateModal = true">
      <GIcon name="add-square" class="mr-0.5" />
      <span>{{ trans('dedicated_servers.create') }}</span>
    </GButton>

    <GButton color="orange" size="middle" :route="{ name: 'admin.client_certificates.index' }">
      <GIcon name="certificate" class="mr-0.5" />
      <span>{{ trans('client_certificates.client_certificates') }}</span>
    </GButton>
  </div>

  <NodesFilters v-model="filters" />

  <div v-if="loading && nodes.length === 0" class="py-10 text-center">
    <Loading />
  </div>

  <div v-else-if="filteredNodes.length === 0" class="py-10">
    <GEmpty :description="trans('servers.empty_list')" />
  </div>

  <div v-else>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      <NodeCard
          v-for="node in pagedNodes"
          :key="node.id"
          :node="node"
          :online="onlineMap.get(String(node.id)) === true"
          :snapshot="metricsSnapshotFor(node.id)"
          @open-details="openDetails"
      />
    </div>

    <div v-if="totalPages > 1" class="mt-6 flex justify-center">
      <n-pagination
          v-model:page="page"
          :page-count="totalPages"
          :page-slot="7"
      />
    </div>
  </div>

  <CreateNodeModal v-model="showCreateModal" />

  <NodeDetailsModal
      :show="modalOpen"
      :node-id="selectedNodeId"
      :node="selectedNode"
      :online="onlineMap.get(String(selectedNodeId)) === true"
      @update:show="onModalShowChange"
      @deleted="onNodeDeleted"
  />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { NPagination } from 'naive-ui'
import { useRoute, useRouter } from 'vue-router'

import { GBreadcrumbs, GIcon, GEmpty, Loading } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import CreateNodeModal from '@/components/blocks/CreateNodeModal.vue'
import NodeCard from '@/components/blocks/NodeCard.vue'
import NodesFilters from '@/components/blocks/NodesFilters.vue'
import NodeDetailsModal from '@/components/blocks/NodeDetailsModal.vue'

import { useNodeListStore } from '@/store/nodeList'
import { useNodesMetricsWebSocket } from '@/composables/useNodesMetricsWebSocket'
import { errorNotification } from '@/parts/dialogs'
import { trans } from '@/i18n/i18n'

const PAGE_SIZE = 24
const SUMMARY_POLL_MS = 30_000

const route = useRoute()
const router = useRouter()
const nodeListStore = useNodeListStore()
const { nodes, loading } = storeToRefs(nodeListStore)

const breadcrumbs = computed(() => [
    { route: '/', text: 'GameAP', icon: 'gameap' },
    { route: { name: 'admin.nodes.index' }, text: trans('sidebar.dedicated_servers') },
])

const filters = ref({ search: '', status: 'all', os: 'all' })
const page = ref(1)
const showCreateModal = ref(false)

const { snapshotFor } = useNodesMetricsWebSocket()

const onlineMap = computed(() => {
    const map = new Map()
    nodeListStore.summaryById.forEach((v, k) => {
        map.set(k, !!v.online)
    })
    return map
})

const filteredNodes = computed(() => {
    const q = filters.value.search.trim().toLowerCase()
    const wantStatus = filters.value.status
    const wantOS = filters.value.os

    return nodes.value.filter((node) => {
        if (q) {
            const inName = node.name?.toLowerCase().includes(q)
            const ips = Array.isArray(node.ip) ? node.ip : []
            const inIp = ips.some(ip => ip.toLowerCase().includes(q))
            if (!inName && !inIp) return false
        }
        if (wantStatus !== 'all') {
            const isOnline = onlineMap.value.get(String(node.id)) === true
            if (wantStatus === 'online' && !isOnline) return false
            if (wantStatus === 'offline' && isOnline) return false
        }
        if (wantOS !== 'all') {
            const os = String(node.os || '').toLowerCase()
            if (wantOS === 'linux' && !(os === '' || os.startsWith('l') || /^(ubu|deb|cen|fed|alm|roc|arc|sus)/.test(os))) return false
            if (wantOS === 'windows' && !os.startsWith('w')) return false
            if (wantOS === 'macos' && !os.startsWith('m')) return false
        }
        return true
    })
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredNodes.value.length / PAGE_SIZE)))

const pagedNodes = computed(() => {
    const start = (page.value - 1) * PAGE_SIZE
    return filteredNodes.value.slice(start, start + PAGE_SIZE)
})

watch(filters, () => {
    page.value = 1
}, { deep: true })

watch(totalPages, (n) => {
    if (page.value > n) page.value = n
})

const modalOpen = computed(() => !!route.query.node)
const selectedNodeId = computed(() => {
    const v = route.query.node
    if (!v) return null
    return Number(Array.isArray(v) ? v[0] : v)
})
const selectedNode = computed(() =>
    selectedNodeId.value ? nodes.value.find(n => n.id === selectedNodeId.value) || null : null,
)

function openDetails(id) {
    router.push({ query: { ...route.query, node: String(id) } })
}

function onModalShowChange(v) {
    if (v) return
    const q = { ...route.query }
    delete q.node
    router.push({ query: q })
}

function onNodeDeleted() {
    fetchAll()
}

function metricsSnapshotFor(id) {
    return snapshotFor(id)
}

async function fetchAll() {
    try {
        await Promise.all([
            nodeListStore.fetchNodesByFilter([]),
            nodeListStore.fetchNodesSummary(),
        ])
    } catch (e) {
        errorNotification(e)
    }
}

let summaryTimer = null

onMounted(async () => {
    await fetchAll()
    summaryTimer = setInterval(() => {
        nodeListStore.fetchNodesSummary().catch(() => {})
    }, SUMMARY_POLL_MS)
})

onBeforeUnmount(() => {
    if (summaryTimer) {
        clearInterval(summaryTimer)
        summaryTimer = null
    }
})
</script>
