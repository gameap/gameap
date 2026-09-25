<script setup>
  import {ref, h} from 'vue'
  import {confirm, errorNotification, notification} from '@/parts/dialogs'
  import {trans} from "@/i18n/i18n";
  import {useAuthStore} from "@/store/auth";
  import axios from "@/config/axios";
  import { GIcon } from '@gameap/ui';
  import GButton from "@/components/GButton.vue";
  import { useTaskWebSocket } from '@/composables/useTaskWebSocket'
  import { createIdempotentRequest } from '@/utils/idempotency'
  import IdempotencyNotice from '@/components/IdempotencyNotice.vue'

  const authStore = useAuthStore();

  const bodyStyle = {
      width: "600px"
  }

  const CLEAR_STATE_TIMEOUT           = 2000; // 2 sec
  const LONG_WAITING_TIME             = 20000; // 20 sec

  const PROGRESS_PERCENT_NULL = 0;
  const PROGRESS_PERCENT_WAITING = 10;
  const PROGRESS_PERCENT_WORKING = 40;
  const PROGRESS_PERCENT_TASK_SUCCESS = 80;
  const PROGRESS_PERCENT_COMPLETE = 100;

  const CHECK_SERVER_STATUS_TRIES = 10;
  const CHECK_SERVER_STATUS_TIMEOUT   = 2000; // 2 sec

  const commandConfiguration = {
      start: {
          title: trans('servers.starting'),
          checkServerStatusAfterTask: true,
          expectedStatus: true,
          successMessage: trans('servers.start_success_msg'),
          failMessage: trans('servers.start_fail_msg'),
      },
      stop: {
          title: trans('servers.stopping'),
          checkServerStatusAfterTask: true,
          expectedStatus: false,
          successMessage: trans('servers.stop_success_msg'),
          failMessage: trans('servers.stop_fail_msg'),
      },
      restart: {
          title: trans('servers.restarting'),
          checkServerStatusAfterTask: true,
          expectedStatus: true,
          successMessage: trans('servers.restart_success_msg'),
          failMessage: trans('servers.restart_fail_msg'),
      },
      update: {
          title: trans('servers.updating'),
          checkServerStatusAfterTask: false,
      },
      install: {
          title: trans('servers.installing'),
          checkServerStatusAfterTask: false,
      },
      reinstall: {
          title: trans('servers.reinstalling'),
          checkServerStatusAfterTask: false,
      }
  }

  const showProgressbar = ref(false);
  const progress = ref(PROGRESS_PERCENT_NULL);
  const progressModalTitle = ref('');
  const progressDetails = ref('');
  const currentTaskId = ref(null);
  const currentCommand = ref(null);
  const submitting = ref(false);
  const showUncertain = ref(false);
  const uncertainCommand = ref(null);

  const props = defineProps([
      'button',
      'buttonColor',
      'buttonSize',
      'icon',
      'text',
      'command',
      'serverId',
  ]);

  // state
  let watchTaskStartedTime;
  let detailedError = false;
  let statusTries = CHECK_SERVER_STATUS_TRIES;
  let clearStateTimer = null;

  useTaskWebSocket(currentTaskId, {
    onStatusChange(status) {
      handleTaskStatus(currentCommand.value, status)
    },
    onComplete(status) {
      handleTaskComplete(currentCommand.value, status)
    },
  })

  function handleTaskStatus(command, status) {
    if (status === 'waiting') {
      progressDetails.value = trans('servers.command_progress_waiting')
      progress.value = PROGRESS_PERCENT_WAITING
      checkLongWaiting()
    } else if (status === 'working') {
      progressDetails.value = trans('servers.command_progress_executed')
      progress.value = PROGRESS_PERCENT_WORKING
      hideAdditionalInfo()
    }
  }

  function handleTaskComplete(command, status) {
    if (status === 'success') {
      progress.value = PROGRESS_PERCENT_TASK_SUCCESS

      if (commandConfiguration[command]?.checkServerStatusAfterTask) {
        progressDetails.value = trans('servers.command_progress_waiting_status')
        setTimeout(watchServerStatus, CHECK_SERVER_STATUS_TIMEOUT, command)
      } else {
        progressDetails.value = ""
        taskSuccess(commandConfiguration[command]?.successMessage)
        setTimeout(clearState, CLEAR_STATE_TIMEOUT)
      }
    } else if (status === 'canceled') {
      progressDetails.value = ""
      progress.value = PROGRESS_PERCENT_NULL
      taskError(trans('gdaemon_tasks.common_canceled_msg'))
    } else {
      progressDetails.value = ""
      progress.value = PROGRESS_PERCENT_COMPLETE
      taskError(trans('gdaemon_tasks.common_error_msg'))
    }
  }

  function run(command) {
      if (submitting.value) {
          return
      }

      // The unanswered attempt keeps its key until the user retries it or
      // starts a new operation; a plain click must not resend it silently.
      if (uncertainCommand.value === command) {
          showUncertain.value = true
          return
      }

      confirm(trans('main.confirm_message'), () => runCommand(command));
  }

  const commandRequests = {}

  function commandRequest(command) {
      commandRequests[command] ??= createIdempotentRequest()

      return commandRequests[command]
  }

  function resetRequest() {
      commandRequest(uncertainCommand.value).reset()
      uncertainCommand.value = null
      showUncertain.value = false
  }

  function retryRequest() {
      showUncertain.value = false
      runCommand(uncertainCommand.value)
  }

  function runCommand(command) {
      if (submitting.value) {
          return
      }

      progress.value = PROGRESS_PERCENT_NULL

      if (authStore.isAdmin) {
          detailedError = true;
      }

      const pending = commandRequest(command)
      const request = pending.prepare()
      submitting.value = true

      axios.post('/api/servers/' + props.serverId + '/' + command, request.data, {headers: request.headers})
          .then(function (response) {
              pending.settle()
              uncertainCommand.value = null

              const taskId = response.data.gdaemonTaskId;

              showProgressbar.value = true
              progressModalTitle.value = commandConfiguration[command].title

              watchTaskStartedTime = (new Date()).getTime();

              clearTimeout(clearStateTimer)
              currentCommand.value = command
              currentTaskId.value = taskId
          }).catch(function (error) {
            pending.settle(error)
            if (pending.pending) {
                uncertainCommand.value = command
            }

            errorNotification(error, () => {
                showUncertain.value = pending.pending
            })
          }).finally(() => {
              submitting.value = false
          });
  }

  function checkLongWaiting() {
      if ((new Date()).getTime() - watchTaskStartedTime > LONG_WAITING_TIME) {
          showAdditionalInfo(trans('gdaemon_tasks.long_waiting_doc'));
      }
  }

  function showAdditionalInfo(text) {
      const additionalInfo = document.querySelector('#additional-info');

      additionalInfo.innerHTML = text;
      additionalInfo.style.display = 'block';
  }

  function hideAdditionalInfo() {
      const additionalInfo = document.querySelector('#additional-info');
      additionalInfo.style.display = 'none';
  }

  function taskError(errorMsg) {
      if (errorMsg === undefined || errorMsg === "") {
          errorMsg = trans('gdaemon_tasks.common_error_msg')
      }

      let content = "";
      if (detailedError && currentTaskId.value) {
          const taskId = currentTaskId.value
          content = () => [
              h('div', {class: 'my-4'}, [
                  h('span', {class: 'mr-2'}, trans('servers.task_see_log')),
                  h(
                      GButton,
                      {color: 'black', size: 'small', onClick: () => { window.location.href = '/admin/gdaemon_tasks/' + taskId; }},
                      () => [
                          h('span', {class: 'inline'}, trans('main.details')),
                          h(GIcon, {name: 'chevron-double-right'})
                      ]
                  )
              ])
          ];
      }

      notification({
        title: errorMsg,
        content: content,
        type: 'error'
      }, () => { location.reload() })

      watchTaskStartedTime = 0;
      hideAdditionalInfo();
      showProgressbar.value = false;

      taskComplete();
  }

  function taskSuccess(msg) {
      hideAdditionalInfo();

      watchTaskStartedTime = 0;

      if (msg === undefined || msg === "") {
          msg = trans('servers.task_success_msg');
      }

      notification({
        title: trans('main.success'),
        content: msg,
        type: 'success'
      }, () => { location.reload() })

      taskComplete();
  }

  function taskComplete() {
      showProgressbar.value = false

      clearStateTimer = setTimeout(clearState, CLEAR_STATE_TIMEOUT);
  }

  function watchServerStatus(command) {
      getServerStatus((serverStatus) => {
          if (serverStatus === commandConfiguration[command].expectedStatus) {
              taskSuccess(commandConfiguration[command].successMessage);
              return;
          }

          if (statusTries <= 0) {
              taskError(commandConfiguration[command].failMessage);
              return;
          }

          statusTries--;
          if (progress.value <= 90) {
              progress.value++;
          }
          setTimeout(watchServerStatus, CHECK_SERVER_STATUS_TIMEOUT, command);
      });
  }

  function getServerStatus(fn) {
      axios.get('/api/servers/' + props.serverId + '/status')
          .then(function (response) {
              fn(response.data.processActive);
          });
  }

  function clearState() {
      clearStateTimer = null;
      progress.value = 0;
      statusTries = CHECK_SERVER_STATUS_TRIES;
      currentTaskId.value = null;
      currentCommand.value = null;
  }

  function progressModalChanged(show) {
      if (!show) {
          showProgressbar.value = false
          clearTimeout(clearStateTimer)
          currentTaskId.value = null
          currentCommand.value = null
      }
  }
</script>

<template>
    <n-modal v-model:show="showUncertain" preset="card" :style="bodyStyle" :title="text">
      <IdempotencyNotice :check-url="'/servers/' + serverId" @reset="resetRequest" />
      <GButton color="green" class="mt-3" data-testid="server-command-retry" @click="retryRequest">
        {{ trans('servers.request_retry') }}
      </GButton>
    </n-modal>

    <n-modal
            v-model:show="showProgressbar"
            class="custom-card"
            preset="card"
            :style="bodyStyle"
            :title="progressModalTitle"
            :bordered="false"
            size="huge"
            :on-update:show="progressModalChanged"
    >
        <div class="progress-info">{{ progressDetails }}</div>
        <n-progress
                type="line"
                :height="24"
                :border-radius="4"
                :percentage="progress"
                :indicator-placement="'inside'"
                processing
        />
        <div id="additional-info" class="mt-3"></div>
        <div v-if="authStore.isAdmin && currentTaskId" class="mt-6">
          <GButton color="black" size="small" :route="'/admin/gdaemon_tasks/' + currentTaskId">
            <span class="inline">{{ trans('main.details') }}</span>
            <GIcon name="chevron-double-right" />
          </GButton>
        </div>
  </n-modal>

  <g-button :class="button" :color="buttonColor" :size="buttonSize" :loading="submitting" @click="run(command)">
    <GIcon :name="icon" />
    <span class="hidden lg:inline">&nbsp;{{ text }}</span>
  </g-button>
</template>

<style>
.progress-info {
    color: silver;
}
</style>
