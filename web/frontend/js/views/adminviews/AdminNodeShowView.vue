<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="blue" size="middle" class="mb-5 mr-1" :route="{name: 'admin.nodes.edit', params: {id: route.params.id}}">
    <i class="fa-solid fa-edit mr-0.5"></i>
    <span>{{ trans('main.edit')}}</span>
  </GButton>

  <GButton color="orange" size="middle" class="mb-5 mr-1" :link="'/api/dedicated_servers/'+route.params.id+'/logs.zip'">
    <i class="fa-solid fa-download mr-0.5"></i>
    <span>{{ trans('dedicated_servers.download_logs')}}</span>
  </GButton>

  <GButton color="green" size="middle" class="mb-5 mr-1" link="/api/dedicated_servers/certificates.zip">
    <i class="fa-solid fa-download mr-0.5"></i>
    <span>{{ trans('dedicated_servers.download_certificates')}}</span>
  </GButton>

  <n-card
      size="small"
      class="mb-3"
      header-class="g-card-header"
      :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
  >
    <Loading v-if="loading"></Loading>
    <n-table v-else :bordered="false" :single-line="true">
      <tbody>
      <tr>
        <td><strong>ID:</strong></td>
        <td>{{ daemonInfo.id }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.name') }}:</strong></td>
        <td>{{ daemonInfo.name }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_api_key') }}:</strong></td>
        <td>{{ daemonInfo.api_key }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_version') }}:</strong></td>
        <td>{{
            daemonInfo.version && daemonInfo.version.version
                ? daemonInfo.version.version + ' (' + daemonInfo.version.compile_date + ')'
                : trans('dedicated_servers.gdaemon_empty_info')
          }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_uptime') }}:</strong></td>
        <td>{{
            daemonInfo.base_info && daemonInfo.base_info.uptime
                ? daemonInfo.base_info.uptime
                : trans('dedicated_servers.gdaemon_empty_info')
          }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_online_servers_count') }}:</strong></td>
        <td>{{
            daemonInfo.base_info && daemonInfo.base_info.online_servers_count
                ? daemonInfo.base_info.online_servers_count
                : trans('dedicated_servers.gdaemon_empty_info')
          }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_working_tasks_count') }}:</strong></td>
        <td>{{
            daemonInfo.base_info && daemonInfo.base_info.working_tasks_count
                ? daemonInfo.base_info.working_tasks_count
                : trans('dedicated_servers.gdaemon_empty_info')
          }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('dedicated_servers.gdaemon_waiting_tasks_count') }}:</strong></td>
        <td>{{
            daemonInfo.base_info && daemonInfo.base_info.waiting_tasks_count
                ? daemonInfo.base_info.waiting_tasks_count
                : trans('dedicated_servers.gdaemon_empty_info')
          }}</td>
      </tr>
      </tbody>
    </n-table>
  </n-card>
</template>

<script setup>
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, onMounted} from "vue"
import {trans} from "../../i18n/i18n"
import {
  NCard,
  NTable,
} from "naive-ui"
import {useNodeStore} from "../../store/node"
import {errorNotification} from "../../parts/dialogs"
import Loading from "../../components/Loading.vue"
import {storeToRefs} from "pinia"
import {useRoute} from "vue-router"
import GButton from "../../components/GButton.vue";

const route = useRoute()

const nodeStore = useNodeStore()

const { daemonInfo, loading } = storeToRefs(nodeStore)

const breadcrumbs = computed(() => {
  let result = [
    {route:'/', text:'GameAP', icon: 'gicon gicon-gameap'},
    {route:{name: 'admin.nodes.index'}, text:trans('dedicated_servers.dedicated_servers')},
  ]

  if (daemonInfo.value.name) {
    result.push({text: daemonInfo.value.name})
  }

  return result
})

onMounted(() => {
  nodeStore.setNodeId(route.params.id)
  nodeStore.fetchDaemonInfo().catch((error) => {
    errorNotification(error)
  })
})



</script>