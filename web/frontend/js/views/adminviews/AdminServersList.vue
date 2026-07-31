<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" :route="{name: 'admin.servers.create'}">
    <GIcon name="add-square" class="mr-0.5" />
    <span>{{ trans('servers.create')}}</span>
  </GButton>

  <GDataTable
      remote
      ref="tableRef"
      :columns="columns"
      :data="serversData"
      :loading="loading"
      :pagination="pagination"
      :scroll-x="isSmallScreen ? 520 : 900"
      @update:page="handlePageChange"
  >
    <template #loading>
      <Loading />
    </template>
    <template #empty>
      <GEmpty :description="trans('servers.empty_list')" />
    </template>
  </GDataTable>
</template>

<script setup>
import { GBreadcrumbs, Loading, GIcon, GDataTable, GEmpty, GGameIcon } from "@gameap/ui"
import {trans} from "../../i18n/i18n"
import GButton from "../../components/GButton.vue"
import {h, onMounted, computed, ref, reactive} from "vue"
import {useServerListStore} from "../../store/serverList"
import {useNodeListStore} from "../../store/nodeList"
import {errorNotification} from "../../parts/dialogs"
import {storeToRefs} from "pinia"
import { RouterLink } from 'vue-router';
import {useIsSmallScreen} from "@/composables/useIsSmallScreen";

const serverListStore = useServerListStore()
const nodeListStore = useNodeListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'admin.servers.index'}, 'text':trans('servers.game_servers')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('servers.name'),
      key: "name"
    },
    {
      title: trans('servers.game'),
      key: "game",
      render: (row) => [
        h(GGameIcon, {game: row.gameCode, class: 'mr-2'}),
        row.game,
      ]
    },
    {
      title: trans('servers.ip_port'),
      key: "ipPort"
    },
    {
      title: trans('servers.dedicated_server'),
      key: "node",
      render: (row) => {
        let node = nodes.value.find(node => node.id === row.nodeId)

        if (!node) {
          return ''
        }

        return h(
            RouterLink,
            {
              to: {name: 'admin.nodes.view', params: {id: node.id}},
              class: "text-info underline hover:no-underline",
            },
            { default: () => node.name },
        )
      }
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
            h(GButton, {
              color: 'blue',
              size: 'small',
              class: 'mr-0.5',
              route: {name: 'admin.servers.edit', params: {id: row.id}},
            }, { default: () => [
              h(GIcon, {name: 'edit'}),
              h("span", {class: 'hidden lg:inline'}, trans('main.edit')),
            ]}),
            h(GButton, {
              color: 'red',
              size: 'small',
              text: trans('main.delete'),
              onClick: () => {onClickDelete(row.id)},
            }, { default: () => [
                h(GIcon, {name: 'delete'}),
                h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
            ]}),
        ]
      },
    }
  ]
}

const {servers, currentPage, perPage, total, lastPage} = storeToRefs(serverListStore)
const {nodes} = storeToRefs(nodeListStore)

const loading = computed(() => {
  return serverListStore.loading || nodeListStore.loading
})

const isSmallScreen = useIsSmallScreen()

const columns = computed(() => {
  const cols = createColumns()
  if (isSmallScreen.value) {
    return cols.filter((col) => !['game', 'node'].includes(col.key))
  }

  return cols
})
const pagination = reactive({
  page: 1,
  pageSize: 30,
  pageCount: 1,
  itemCount: 0,
});

onMounted(() => {
  fetchServers()
  fetchNodes()
})

const fetchServers = () => {
  return serverListStore.fetchServersByFilter({
    page: pagination.page,
    perPage: pagination.pageSize,
  })
    .then(() => {
      pagination.pageCount = lastPage.value
      pagination.itemCount = total.value
      pagination.page = currentPage.value
      pagination.pageSize = perPage.value
    })
    .catch((error) => {
      errorNotification(error)
    })
}

const fetchNodes = () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
}

const handlePageChange = (page) => {
  pagination.page = page
  fetchServers()
}

const serversData = computed(() => {
  let result = []

  servers.value.forEach((server) => {
    result.push({
      id: server.id,
      name: server.name,
      game: server.game.name,
      gameCode: server.game.code,
      nodeId: server.ds_id,
      ipPort: server.server_ip + ':' + server.server_port,
    })
  })

  return result
})

const onClickDelete = (id) => {
  let deleteFiles = false

  window.$dialog.success({
    title: trans('servers.delete_confirm_msg'),
    content: () => h('div', {class: "mt-4 mb-4"}, [
      h('input', {
        type: 'checkbox',
        id: 'delete-files-checkbox',
        onChange: () => {deleteFiles = true;},
      }),
      h('label', {class: 'ms-1', for: 'delete-files-checkbox'}, trans('servers.delete_files')),
    ]),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteByID(id, deleteFiles)
    },
    onNegativeClick: () => {}
  });
}

const deleteByID = (id, deleteFiles) => {
  serverListStore.deleteById(id, deleteFiles).
    catch((error) => {
      errorNotification(error)
    }).
    finally(() => {
      fetchServers()
  })
}

</script>
