<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5 mr-1" :route="{name: 'admin.nodes.create'}">
    <i class="fa fa-plus-square mr-0.5"></i>
    <span>{{ trans('dedicated_servers.create')}}</span>
  </GButton>

  <GButton color="orange" size="middle" class="mb-5" :route="{name: 'admin.client_certificates.index'}">
    <i class="fa-solid fa-certificate mr-0.5"></i>
    <span>{{ trans('client_certificates.client_certificates')}}</span>
  </GButton>

  <n-data-table
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="nodesData"
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
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, h, ref, onMounted} from "vue"
import {trans} from "../../i18n/i18n"
import GButton from "../../components/GButton.vue"
import {useNodeListStore} from "../../store/nodeList"
import {errorNotification, notification} from "../../parts/dialogs"
import Loading from "../../components/Loading.vue"
import {storeToRefs} from "pinia"
import { join } from "lodash-es"
import {
  NEmpty,
  NDataTable,
} from "naive-ui"

const nodeListStore = useNodeListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.nodes.index'}, 'text':trans('sidebar.dedicated_servers')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('dedicated_servers.name'),
      key: "name"
    },
    {
      title: trans('dedicated_servers.location'),
      key: "location"
    },
    {
      title: trans('dedicated_servers.provider'),
      key: "provider"
    },
    {
      title: trans('dedicated_servers.ip'),
      key: "ip",
      render(row) {
        return join(row.ip, ', ')
      }
    },
    {
      title: trans('dedicated_servers.os'),
      key: "os"
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'green',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.nodes.view', params: {id: row.id}},
          }, [
            h("i", {class: 'fa-solid fa-eye'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.view')),
          ]),
          h(GButton, {
            color: 'blue',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.nodes.edit', params: {id: row.id}},
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

const {loading, nodes} = storeToRefs(nodeListStore)

const columns = ref(createColumns())
const pagination = {
  pageSize: 20,
};

onMounted(() => {
  fetchNodes()
})

const fetchNodes = () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
}

const nodesData = computed(() => {
  return nodes.value.map((node) => {
    return {
      id: node.id,
      name: node.name,
      location: node.location,
      provider: node.provider,
      ip: node.ip,
      os: node.os,
    }
  })
})

const onClickDelete = (id) => {
  window.$dialog.success({
    title: trans('dedicated_servers.delete_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      nodeListStore.deleteNode(id).then(() => {
        notification({
          content: trans('dedicated_servers.delete_success_msg'),
          type: "success",
        }, () => {
          fetchNodes()
        })
      }).catch((error) => {
        errorNotification(error)
      })
    },
    onNegativeClick: () => {}
  })
}

</script>