<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <div class="mb-1 w-2/3">
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
          v-model:value="search.nodes"
          :options="nodeOptions"
          :placeholder="trans('dedicated_servers.dedicated_servers')"
          @update:value="onUpdateFilters"
      />

      <n-button @click="clearFilters" type="error" :disabled="!isFiltersSet()" ghost>
        <i class="fa fa-eraser"></i><span class="hidden lg:inline">&nbsp;{{ trans('main.clear') }}</span>
      </n-button>
    </n-input-group>
  </div>

  <n-data-table
      remote
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="listData"
      :loading="loading"
      :pagination="pagination"
      @update:page="handlePageChange"
  >
    <template #loading>
      <Loading />
    </template>
  </n-data-table>
</template>

<script setup>
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, h, ref, reactive, onMounted} from "vue"
import {
  NButton,
  NSelect,
  NInputGroup,
  NDataTable,
} from "naive-ui"
import {trans} from "../../i18n/i18n"
import Loading from "../../components/Loading.vue"
import {useDaemonTaskListStore} from "../../store/daemonTaskList"
import {useNodeListStore} from "../../store/nodeList"
import {storeToRefs} from "pinia"
import GButton from "../../components/GButton.vue";
import GStatusBadge from "../../components/GStatusBadge.vue";
import {errorNotification} from "../../parts/dialogs"

const daemonTaskListStore = useDaemonTaskListStore()
const nodeListStore = useNodeListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.gdaemon_tasks.index'}, 'text':trans('gdaemon_tasks.gdaemon_tasks')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('gdaemon_tasks.task'),
      key: "task",
      render(row) {
        return renderTaskNameWithIcon(row.task, trans('gdaemon_tasks.'+row.task))
      },
    },
    {
      title: trans('gdaemon_tasks.status'),
      key: "status",
      render(row) {
        return h(GStatusBadge, {status: row.status, text: trans('gdaemon_tasks.status_' + row.status)})
      },
    },
    {
      title: trans('servers.dedicated_server'),
      render(row) {
        let node = nodes.value.find((node) => node.id === row.nodeId)
        return node ? node.name : ''
      },
    },
    {
      title: trans('gdaemon_tasks.created'),
      key: "createdAt"
    },
    {
      title: trans('gdaemon_tasks.updated'),
      key: "updatedAt"
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'green',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.gdaemon_tasks.output', params: {id: row.id}},
          }, [
            h("i", {class: 'fa-solid fa-eye'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.view')),
          ]),
        ]
      },
    }
  ]
}

const {daemonTaskList, currentPage, total} = storeToRefs(daemonTaskListStore)
const {nodes} = storeToRefs(nodeListStore)

const columns = ref(createColumns())
const pagination = reactive({
  page: currentPage,
  pageSize: 30,
})

const loading = computed(() => {
  return daemonTaskListStore.loading || nodeListStore.loading
})

onMounted(() => {
  currentPage.value = 1

  fetchTasks()
  fetchNodes()
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

const listData = computed(() => {
  return daemonTaskList.value.map((task) => {
    return {
      id: task.id,
      task: task.task,
      status: task.status,
      nodeId: task.dedicated_server_id,
      createdAt: (new Date(task.created_at)).toLocaleString(),
      updatedAt: (new Date(task.updated_at)).toLocaleString(),
    }
  })
})

const search = ref({
  tasks: null,
  statuses: null,
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
      return [h("i", {class: "fa-solid fa-play mr-2"}), taskName]
    case 'gsstop':
      return [h("i", {class: "fa-solid fa-stop mr-2"}), taskName]
    case 'gsrest':
      return [h("i", {class: "fa-solid fa-redo mr-2"}), taskName]
    case 'gsupd':
      return [h("i", {class: "fa-solid fa-refresh mr-2"}), taskName]
    case 'gsinst':
      return [h("i", {class: "fa-solid fa-download mr-2"}), taskName]
    case 'gsdel':
      return [h("i", {class: "fa-solid fa-trash mr-2"}), taskName]
    case 'gsmove':
      return [h("i", {class: "fa-solid fa-dolly mr-2"}), taskName]
    case 'cmdexec':
      return [h("i", {class: "fa-solid fa-terminal mr-2"}), taskName]
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

const handlePageChange = (page) => {
  currentPage.value = page
  fetchTasks()
}

const onUpdateFilters = () => {
  currentPage.value = 1
  fetchTasks()
}

const isFiltersSet = () => {
  return search.value.tasks || search.value.statuses || search.value.nodes
}

const clearFilters = () => {
  search.value.tasks = null
  search.value.statuses = null
  search.value.nodes = null
  currentPage.value = 1

  fetchTasks()
}
</script>