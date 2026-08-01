<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <Loading v-if="loading"></Loading>
  <div :class="loading ? 'hidden' : ''">
    <div v-if="canCancel" class="mb-3">
      <GButton color="red" size="middle" @click="onCancelClick">
        {{ trans('gdaemon_tasks.cancel') }}
      </GButton>
    </div>

    <GTable>
      <tbody>
      <tr>
        <td><strong>{{ trans('gdaemon_tasks.task') }}:</strong></td>
        <td>{{ trans('gdaemon_tasks.'+task.task) }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('gdaemon_tasks.status') }}:</strong></td>
        <td><GStatusBadge v-if="displayStatus" :status="displayStatus" /></td>
      </tr>
      <tr>
        <td><strong>{{ trans('gdaemon_tasks.created') }}:</strong></td>
        <td>{{ task.created_at }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('gdaemon_tasks.created') }}:</strong></td>
        <td>{{ task.updated_at }}</td>
      </tr>
      </tbody>
    </GTable>

    <div class="w-full mt-3">
      <div class="coding inverse-toggle px-5 pt-4 shadow-lg text-terminal text-sm font-mono subpixel-antialiased
              bg-terminal pb-6 pt-4 rounded-lg leading-normal overflow-hidden">
        <div class="top mb-2 flex">
          <div class="h-3 w-3 bg-red-500 rounded-full"></div>
          <div class="ml-2 h-3 w-3 bg-orange-300 rounded-full"></div>
          <div class="ml-2 h-3 w-3 bg-lime-500 rounded-full"></div>
        </div>
        <div class="whitespace-pre-wrap mt-4 flex ">
          {{ output }}
        </div>
      </div>
    </div>
  </div>

</template>

<script setup>
import { GBreadcrumbs, GStatusBadge, Loading, GTable } from "@gameap/ui"
import GButton from "@/components/GButton.vue"
import {computed, onMounted} from "vue"
import {trans} from "../../i18n/i18n"
import {confirm, errorNotification, notification} from "../../parts/dialogs"
import {useRoute} from "vue-router"
import {storeToRefs} from "pinia"
import {useDaemonTaskStore} from "../../store/daemonTask"
import { replace } from "lodash-es"
import { useTaskWebSocket } from '@/composables/useTaskWebSocket'

const daemonTaskStore = useDaemonTaskStore()
const route = useRoute()

const { taskStatus, taskOutput } = useTaskWebSocket(route.params.id)

const breadcrumbs = computed(() => {
  let result = [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'admin.gdaemon_tasks.index'}, 'text':trans('gdaemon_tasks.gdaemon_tasks')},
  ]

  if (task.value.id) {
    result.push({
      route: {name: 'admin.gdaemon_tasks.output', params: {id: task.value.id}},
      text:trans('gdaemon_tasks.'+task.value.task),
    })
  }

  return result
})

const {loading, task} = storeToRefs(daemonTaskStore)

const displayStatus = computed(() => taskStatus.value || task.value.status)
const canCancel = computed(() => displayStatus.value === 'waiting')

const onCancelClick = () => {
  confirm(trans('main.confirm_message'), async () => {
    try {
      await daemonTaskStore.cancelTask()
      notification({
        type: 'success',
        content: trans('gdaemon_tasks.canceled_success_msg'),
      })
    } catch (error) {
      errorNotification(error)
    }
  })
}

onMounted(() => {
  daemonTaskStore.setTaskId(route.params.id)
  daemonTaskStore.fetchTaskOutput()
    .catch((error) => {
      errorNotification(error)
    })
})

const output = computed(() => {
  if (!taskOutput.value) {
    return ''
  }

  return replace(taskOutput.value, /(\r\n|\n|\r)/gm, "\n")
})
</script>
