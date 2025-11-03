<script setup>
  import {ref, h} from 'vue'
  import {alert, confirm} from '@/parts/dialogs'
  import {trans} from "@/i18n/i18n";
  import {useAuthStore} from "@/store/auth";
  import axios from "@/config/axios";
  import GButton from "@/components/GButton.vue";

  const authStore = useAuthStore();

  const bodyStyle = {
      width: "600px"
  }

  const WATCH_TASK_TIMEOUT            = 2000; // 2 sec
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
  let watchTaskData = {}
  let watchTaskStartedTime;
  let watchTaskStopped = false;
  let detailedError = false;
  let statusTries = CHECK_SERVER_STATUS_TRIES;

  function run(command) {
      confirm(trans('main.confirm_message'), () => runCommand(command));
  }

  function runCommand(command) {
      progress.value = PROGRESS_PERCENT_NULL

      if (authStore.isAdmin) {
          detailedError = true;
      }

      axios.post('/api/servers/' + props.serverId + '/' + command)
          .then(function (response) {
              const taskId = response.data.gdaemonTaskId;

              showProgressbar.value = true
              progressModalTitle.value = commandConfiguration[command].title

              watchTaskStartedTime = (new Date()).getTime();
              watchTaskStopped = false;
              watchTask(command, taskId);
          }).catch(function (error) {
            alert(error.response.data.message, function() {
                location.reload();
            });
      });
  }

  function watchTask(command, id) {
      getTask(id);
      let checkAgain = true;

      if (Object.keys(watchTaskData).length !== 0) {
          if (watchTaskData.status === 'waiting') {
              progressDetails.value = trans('servers.command_progress_waiting');
              progress.value = PROGRESS_PERCENT_WAITING;
              checkLongWaiting();
          } else if (watchTaskData.status === 'working') {
              progressDetails.value = trans('servers.command_progress_executed');
              progress.value = PROGRESS_PERCENT_WORKING;
              hideAdditionalInfo();
          } else if (watchTaskData.status === 'success') {
              checkAgain = false;
              progress.value = PROGRESS_PERCENT_TASK_SUCCESS;

              if (commandConfiguration[command].checkServerStatusAfterTask) {
                  progressDetails.value = trans('servers.command_progress_waiting_status');
                  setTimeout(watchServerStatus, CHECK_SERVER_STATUS_TIMEOUT, command);
              } else {
                  progressDetails.value = "";
                  taskSuccess(commandConfiguration[command].successMessage);
                  setTimeout(clearState, CLEAR_STATE_TIMEOUT);
              }
          } else if (watchTaskData.status === 'canceled') {
              progressDetails.value = "";
              checkAgain = false;
              progress.value = PROGRESS_PERCENT_NULL;
              taskError(trans('gdaemon_tasks.common_canceled_msg'));
          } else if (watchTaskData.status === 'error') {
              progressDetails.value = "";
              checkAgain = false;
              progress.value = PROGRESS_PERCENT_COMPLETE;
              taskError(trans('gdaemon_tasks.common_error_msg'));
          } else {
              progressDetails.value = "";
              checkAgain = false;
              progress.value = PROGRESS_PERCENT_COMPLETE;
              taskError(trans('gdaemon_tasks.common_error_msg'));
          }
      }

      if (checkAgain && !watchTaskStopped) {
          setTimeout(watchTask, WATCH_TASK_TIMEOUT, command, id);
      }
  }

  function getTask(id) {
      axios.get('/api/gdaemon_tasks/' + id)
          .then(function (response) {
              watchTaskData = response.data;
          }).catch(function(error) {
              taskError(error.response.data.message);
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

      watchTaskStopped = true;

      let content = "";
      if (detailedError) {
          content = () => [
              h('div', [
                  h(
                      'a',
                      {href: "/admin/gdaemon_tasks/" + watchTaskData.id},
                      trans('servers.task_see_log'),
                  )
              ])
          ];
      }
      alert(errorMsg, () => { location.reload() }, content);

      watchTaskStartedTime = 0;
      hideAdditionalInfo();
      showProgressbar.value = false;

      taskComplete();
  }

  function taskSuccess(msg) {
      watchTaskStopped = true;
      hideAdditionalInfo();

      watchTaskStartedTime = 0;

      if (msg === undefined || msg === "") {
          msg = trans('servers.task_success_msg');
      }

      alert(msg, () => { location.reload() });

      taskComplete();
  }

  function taskComplete() {
      showProgressbar.value = false

      setTimeout(clearState, CLEAR_STATE_TIMEOUT);
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
      progress.value = 0;
      watchTaskData = {};
      statusTries = CHECK_SERVER_STATUS_TRIES;
  }

  function progressModalChanged(show) {
      if (!show) {
          showProgressbar.value = false
          watchTaskStopped = true
      }
  }
</script>

<template>
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
  </n-modal>

  <g-button :class="button" :color="buttonColor" :size="buttonSize" @click="run(command)">
    <i :class="icon"></i>
    <span class="hidden lg:inline">&nbsp;{{ text }}</span>
  </g-button>
</template>

<style>
.progress-info {
    color: silver;
}
</style>
