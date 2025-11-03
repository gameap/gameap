<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" :route="{name: 'admin.servers.create'}">
    <i class="fa fa-plus-square mr-0.5"></i>
    <span>{{ trans('servers.create')}}</span>
  </GButton>

  <n-data-table
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="serversData"
      :loading="loading"
      :pagination="pagination"
  >
    <template #loading>
      <Loading />
    </template>
    <template #empty>
      <n-empty :description="trans('servers.empty_list')">
      </n-empty>
    </template>
  </n-data-table>
</template>

<script setup>
import {trans} from "../../i18n/i18n"
import Loading from "../../components/Loading.vue"
import GButton from "../../components/GButton.vue"
import {h, onMounted, computed, ref} from "vue"
import {useServerListStore} from "../../store/serverList"
import {useNodeListStore} from "../../store/nodeList"
import {errorNotification} from "../../parts/dialogs"
import {storeToRefs} from "pinia"
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {
  NDataTable,
  NEmpty,
} from "naive-ui"
import GameIcon from "../../components/GameIcon.vue";
import { RouterLink } from 'vue-router';

const serverListStore = useServerListStore()
const nodeListStore = useNodeListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
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
        h(GameIcon, {game: row.gameCode, class: 'mr-2'}),
        row.game,
      ]
    },
    {
      title: trans('servers.ip_port'),
      key: "ipPort"
    },
    {
      title: trans('servers.dedicated_server'),
      render: (row) => {
        let node = nodes.value.find(node => node.id === row.nodeId)

        // return node.name

        return h(
            RouterLink,
            {
              to: {name: 'admin.nodes.view', params: {id: node.id}},
              class: "text-blue-600 underline dark:text-blue-500 hover:no-underline",
            },
            node.name,
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
            }, [
              h("i", {class: 'fa-solid fa-pen-to-square'}),
              h("span", {class: 'hidden lg:inline'}, trans('main.edit')),
            ]),
            h(GButton, {
              color: 'red',
              size: 'small',
              text: trans('main.delete'),
              onClick: () => {onClickDelete(row.id)},
            }, [
                h("i", {class: 'fa-solid fa-trash'}),
                h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
            ]),
        ]
      },
    }
  ]
}

const {servers} = storeToRefs(serverListStore)
const {nodes} = storeToRefs(nodeListStore)

const loading = computed(() => {
  return serverListStore.loading || nodeListStore.loading
})

const columns = ref(createColumns())
const pagination = {
  pageSize: 20,
};

onMounted(() => {
  fetchServers()
  fetchNodes()
})

const fetchServers = () => {
  serverListStore.fetchServersByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
}

const fetchNodes = () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
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