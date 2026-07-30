<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <div class="mb-1 w-full lg:w-2/3">
    <n-input-group>
      <n-select
          multiple
          v-model:value="search.tasks"
          :options="taskOptions"
          :placeholder="trans('gdaemon_tasks.task')"
          @update:value="onUpdateFilters"
          :render-label="renderTaskOptionLabel"
      />
      <n-select
          multiple
          v-model:value="search.statuses"
          :options="statusOptions"
          :placeholder="trans('gdaemon_tasks.status')"
          @update:value="onUpdateFilters"
          :render-label="renderStatusOptionLabel"
          :render-tag="renderStatusOptionTag"
      />
      <n-select
          multiple
          v-model:value="search.servers"
          :options="serverOptions"
          :placeholder="trans('servers.game_servers')"
          @update:value="onUpdateFilters"
      />
      <n-select
          multiple
          v-model:value="search.nodes"
          :options="nodeOptions"
          :placeholder="trans('dedicated_servers.dedicated_servers')"
          @update:value="onUpdateFilters"
      />

      <n-button @click="clearFilters" type="error" :disabled="!isFiltersSet()" ghost>
        <GIcon name="eraser" /><span class="hidden lg:inline">&nbsp;{{ trans('main.clear') }}</span>
      </n-button>
    </n-input-group>
  </div>

  <GDataTable
      remote
      ref="tableRef"
      :columns="columns"
      :data="listData"
      :loading="loading"
      :pagination="pagination"
      :scroll-x="800"
      @update:page="handlePageChange"
  >
    <template #loading>
      <Loading />
    </template>
  </GDataTable>
</template>

<script setup>
import { GBreadcrumbs, GStatusBadge, Loading, GIcon, GDataTable } from "@gameap/ui"
import {computed, h, ref, reactive, onMounted, onUnmounted} from "vue"
import {
  NButton,
  NSelect,
  NInputGroup,
} from "naive-ui"
import {trans} from "@/i18n/i18n"
import {useDaemonTaskListStore} from "@/store/daemonTaskList"
import {useNodeListStore} from "@/store/nodeList"
import {useServerListStore} from "@/store/serverList"
import {RouterLink} from "vue-router"
import {storeToRefs} from "pinia"
import GButton from "@/components/GButton.vue";
import {errorNotification} from "@/parts/dialogs"

const daemonTaskListStore = useDaemonTaskListStore()
const nodeListStore = useNodeListStore()
const serverListStore = useServerListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.gdaemon_tasks.index'}, 'text':trans('gdaemon_tasks.gdaemon_tasks')},
  ]
})

const isSmallScreen = ref(window.innerWidth < 1024)

const handleResize = () => {
  isSmallScreen.value = window.innerWidth < 1024
}

const formatDateTime = (value) => {
  if (!value) {
    return ''
  }
  const d = new Date(value)
  if (isNaN(d.getTime())) {
    return ''
  }
  const pad = (n) => String(n).padStart(2, '0')

  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

const columns = computed(() => {
  const cols = [
    {
      title: trans('gdaemon_tasks.task'),
      key: "task",
      ellipsis: { tooltip: true },
      render(row) {
        return renderTaskNameWithIcon(row.task, trans('gdaemon_tasks.'+row.task))
      },
    },
    {
      title: trans('gdaemon_tasks.status'),
      key: "status",
      className: "whitespace-nowrap",
      width: 120,
      render(row) {
        return h(GStatusBadge, {
          status: row.status,
          text: trans('gdaemon_tasks.status_' + row.status),
        })
      },
    },
    {
      title: trans('servers.game_server'),
      ellipsis: { tooltip: true },
      render(row) {
        if (!row.serverId) {
          return ''
        }
        const server = servers.value.find((server) => server.id === row.serverId)
        if (!server) {
          return ''
        }

        return h(
          RouterLink,
          {
            to: {name: 'servers.control', params: {id: server.id}},
            class: "text-info underline hover:no-underline",
          },
          { default: () => server.name },
        )
      },
    },
    {
      title: trans('servers.dedicated_server'),
      ellipsis: { tooltip: true },
      render(row) {
        const node = nodes.value.find((node) => node.id === row.nodeId)

        return node ? node.name : ''
      },
    },
    {
      title: trans('gdaemon_tasks.created'),
      key: "createdAt",
      className: "whitespace-nowrap",
      width: 160,
    },
  ]

  if (!isSmallScreen.value) {
    cols.push({
      title: trans('gdaemon_tasks.updated'),
      key: "updatedAt",
      className: "whitespace-nowrap",
      width: 160,
    })
  }

  cols.push({
    title: trans('main.actions'),
    className: "whitespace-nowrap",
    render(row) {
      return [
        h(GButton, {
          color: 'green',
          size: 'small',
          class: 'mr-0.5',
          route: {name: 'admin.gdaemon_tasks.output', params: {id: row.id}},
        }, { default: () => [
          h(GIcon, {name: 'view'}),
          h("span", {class: 'hidden lg:inline'}, trans('main.view')),
        ]}),
      ]
    },
  })

  return cols
})

const {daemonTaskList, currentPage, total} = storeToRefs(daemonTaskListStore)
const {nodes} = storeToRefs(nodeListStore)
const {servers} = storeToRefs(serverListStore)

const pagination = reactive({
  page: currentPage,
  pageSize: 30,
})

const loading = computed(() => {
  return daemonTaskListStore.loading || nodeListStore.loading || serverListStore.loading
})

onMounted(() => {
  currentPage.value = 1

  window.addEventListener('resize', handleResize)

  fetchTasks()
  fetchNodes()
  fetchServers()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

const fetchTasks = () => {
  let filter = {
    page: currentPage.value,
  }

  if (search.value.tasks) {
    filter.tasks = search.value.tasks
  }

  if (search.value.statuses) {
    filter.statuses = search.value.statuses
  }

  if (search.value.servers) {
    filter.servers = search.value.servers
  }

  if (search.value.nodes) {
    filter.nodes = search.value.nodes
  }

  daemonTaskListStore.fetchTasksByFilter(filter).then(() => {
    pagination.pageCount = total.lastPage
    pagination.itemCount = total.value
  }).catch((error) => {
    errorNotification(error)
  })
}

const fetchNodes = () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
}

const fetchServers = () => {
  serverListStore.fetchServersByFilter({perPage: 1000}).
  catch((error) => {
    errorNotification(error)
  })
}

const listData = computed(() => {
  return daemonTaskList.value.map((task) => {
    return {
      id: task.id,
      task: task.task,
      status: task.status,
      serverId: task.server_id,
      nodeId: task.dedicated_server_id,
      createdAt: formatDateTime(task.created_at),
      updatedAt: formatDateTime(task.updated_at),
    }
  })
})

const search = ref({
  tasks: null,
  statuses: null,
  servers: null,
  nodes: null,
})

const taskOptions = [
  {
    label: trans('gdaemon_tasks.gsstart'),
    value: 'gsstart',
  },
  {
    label: trans('gdaemon_tasks.gsstop'),
    value: 'gsstop',
  },
  {
    label: trans('gdaemon_tasks.gsrest'),
    value: 'gsrest',
  },
  {
    label: trans('gdaemon_tasks.gsupd'),
    value: 'gsupd',
  },
  {
    label: trans('gdaemon_tasks.gsinst'),
    value: 'gsinst',
  },
  {
    label: trans('gdaemon_tasks.gsdel'),
    value: 'gsdel',
  },
  {
    label: trans('gdaemon_tasks.gsmove'),
    value: 'gsmove',
  },
  {
    label: trans('gdaemon_tasks.cmdexec'),
    value: 'cmdexec',
  },
]

const statusOptions = [
  {
    label: trans('gdaemon_tasks.status_waiting'),
    value: 'waiting',
  },
  {
    label: trans('gdaemon_tasks.status_working'),
    value: 'working',
  },
  {
    label: trans('gdaemon_tasks.status_error'),
    value: 'error',
  },
  {
    label: trans('gdaemon_tasks.status_success'),
    value: 'success',
  },
  {
    label: trans('gdaemon_tasks.status_canceled'),
    value: 'canceled',
  },
]

const renderTaskOptionLabel = (option) => {
  return renderTaskNameWithIcon(option.value, option.label)
}

const renderTaskNameWithIcon = (taskCode, taskName) => {
  switch (taskCode) {
    case 'gsstart':
      return [h(GIcon, {name: "play", class: "mr-2"}), taskName]
    case 'gsstop':
      return [h(GIcon, {name: "stop", class: "mr-2"}), taskName]
    case 'gsrest':
      return [h(GIcon, {name: "restart", class: "mr-2"}), taskName]
    case 'gsupd':
      return [h(GIcon, {name: "refresh", class: "mr-2"}), taskName]
    case 'gsinst':
      return [h(GIcon, {name: "download", class: "mr-2"}), taskName]
    case 'gsdel':
      return [h(GIcon, {name: "delete", class: "mr-2"}), taskName]
    case 'gsmove':
      return [h(GIcon, {name: "move", class: "mr-2"}), taskName]
    case 'cmdexec':
      return [h(GIcon, {name: "terminal", class: "mr-2"}), taskName]
    default:
      return taskName
  }
}

const renderStatusOptionLabel = (option) => {
  return h(GStatusBadge, {status: option.value, text: option.label })
}

const renderStatusOptionTag = (item) => {
  return h(GStatusBadge, {status: item.option.value, text: item.option.label })
}

const nodeOptions = computed(() => {
  return nodes.value.map((node) => {
    return {
      label: node.name,
      value: node.id,
    }
  })
})

const serverOptions = computed(() => {
  return servers.value.map((server) => {
    return {
      label: server.name,
      value: server.id,
    }
  })
})

const handlePageChange = (page) => {
  currentPage.value = page
  fetchTasks()
}

const onUpdateFilters = () => {
  currentPage.value = 1
  fetchTasks()
}

const isFiltersSet = () => {
  return search.value.tasks || search.value.statuses || search.value.servers || search.value.nodes
}

const clearFilters = () => {
  search.value.tasks = null
  search.value.statuses = null
  search.value.servers = null
  search.value.nodes = null
  currentPage.value = 1

  fetchTasks()
}
</script>