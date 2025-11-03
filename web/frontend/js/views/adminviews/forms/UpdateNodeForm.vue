<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <n-tabs type="line" class="flex justify-between" animated>
        <n-tab-pane name="main">
          <template #tab>
            {{ trans('dedicated_servers.main') }}
          </template>

          <div class="flex flex-wrap mt-2">
            <div class="md:w-1/2 pr-4">
              <n-card
                  :title="trans('games.basic_info')"
                  size="small"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
              >
                <n-form-item :label="trans('labels.name')" path="name">
                  <n-input
                      v-model:value="form.name"
                      type="text"
                  />
                </n-form-item>

                <n-form-item :label="trans('labels.enabled')" path="enabled">
                  <n-switch
                      v-model:value="form.enabled"
                  >
                  </n-switch>
                </n-form-item>

                <n-form-item :label="trans('labels.os')" path="os">
                  <n-select
                      v-model:value="form.os"
                      :options="osOptions"
                      :render-label="renderLabel"
                  />
                </n-form-item>

                <div class="flex flex-wrap w-full">
                  <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.location')" path="location">
                    <n-input
                        v-model:value="form.location"
                        type="text"
                    />
                  </n-form-item>

                  <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.provider')" path="provider">
                    <n-input
                        v-model:value="form.provider"
                        type="text"
                    />
                  </n-form-item>
                </div>

                <div class="flex flex-wrap w-full">
                  <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.work_path')" path="workPath">
                    <n-input
                        v-model:value="form.workPath"
                        type="text"
                    />
                  </n-form-item>

                  <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.steamcmd_path')" path="steamcmdPath">
                    <n-input
                        v-model:value="form.steamcmdPath"
                        type="text"
                    />
                  </n-form-item>
                </div>
              </n-card>
            </div>

            <div class="md:w-1/2 pl-4">
              <n-card
                  :title="trans('dedicated_servers.ip_list')"
                  size="small"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
              >
                <InputTextList
                    v-model="form.ip"
                    name="ip"
                    label="IP"
                />
              </n-card>
            </div>
          </div>
        </n-tab-pane>

        <n-tab-pane name="daemon">
          <template #tab>
            Daemon
          </template>

          <n-card
              size="small"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
          >
            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.gdaemon_host')" path="gdaemonHost">
                <n-input
                    v-model:value="form.gdaemonHost"
                    type="text"
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.gdaemon_port')" path="gdaemonPort">
                <n-input
                    v-model:value="form.gdaemonPort"
                    type="text"
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full mt-4">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.gdaemon_login')" path="gdaemonLogin">
                <n-input
                    v-model:value="form.gdaemonLogin"
                    type="text"
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.gdaemon_password')" path="gdaemonPassword">
                <n-input
                    v-model:value="form.gdaemonPassword"
                    type="password"
                    show-password-on="click"
                    :input-props="{ autocomplete: 'one-time-code' }"
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.client_certificate_id')" path="clientCertificateId">
                <n-select
                    v-model:value="form.clientCertificateId"
                    :options="clientCertificateOptions"
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.gdaemon_server_cert')" path="serverCertificate">
                <n-upload
                    :max="1"
                    :multiple="false"
                    :default-upload="false"
                    @change="handleChangeServerCertificate"
                >
                  <n-button class="mt-2">{{ trans('main.upload_file') }}</n-button>
                </n-upload>
              </n-form-item>
            </div>
          </n-card>
        </n-tab-pane>

        <n-tab-pane name="scripts">
          <template #tab>
            {{ trans('dedicated_servers.scripts') }}
          </template>

          <n-card
              size="small"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
          >
            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_install')" path="scriptInstall">
                <n-input
                    v-model:value="form.scriptInstall"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_reinstall')" path="scriptReinstall">
                <n-input
                    v-model:value="form.scriptReinstall"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_update')" path="scriptUpdate">
                <n-input
                    v-model:value="form.scriptUpdate"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_start')" path="scriptStart">
                <n-input
                    v-model:value="form.scriptStart"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_pause')" path="scriptPause">
                <n-input
                    v-model:value="form.scriptPause"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_unpause')" path="scriptUnpause">
                <n-input
                    v-model:value="form.scriptUnpause"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_stop')" path="scriptStop">
                <n-input
                    v-model:value="form.scriptStop"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_kill')" path="scriptKill">
                <n-input
                    v-model:value="form.scriptKill"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_restart')" path="scriptRestart">
                <n-input
                    v-model:value="form.scriptRestart"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_status')" path="scriptStatus">
                <n-input
                    v-model:value="form.scriptStatus"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_get_console')" path="scriptGetConsole">
                <n-input
                    v-model:value="form.scriptGetConsole"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_send_command')" path="scriptSendCommand">
                <n-input
                    v-model:value="form.scriptSendCommand"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <div class="flex flex-wrap w-full">
              <n-form-item class="md:w-1/2 pr-2" :label="trans('labels.script_delete')" path="scriptDelete">
                <n-input
                    v-model:value="form.scriptDelete"
                    type="text"
                    placeholder=""
                />
              </n-form-item>

              <n-form-item class="md:w-1/2 pl-2" :label="trans('labels.script_stats')" path="scriptStats">
                <n-input
                    v-model:value="form.scriptStats"
                    type="text"
                    placeholder=""
                />
              </n-form-item>
            </div>

            <template #footer>
              <div class="flex flex-wrap ">
                <div class="md:w-1/2 pr-4 pl-4">
                  <h5 class="text-xl font-bold dark:text-white">{{ trans('dedicated_servers.game_server_shortcodes') }}</h5>
                  <ul class="ml-1 space-y-1 list-disc list-inside">
                    <li>{host} — {{ trans('dedicated_servers.d_shortcodes_host') }}</li>
                    <li>{port} — {{ trans('dedicated_servers.d_shortcodes_port') }}</li>
                    <li>{query_port} — {{ trans('dedicated_servers.d_shortcodes_query_port') }}</li>
                    <li>{rcon_port} — {{ trans('dedicated_servers.d_shortcodes_rcon_port') }}</li>
                    <li>{dir} — {{ trans('dedicated_servers.d_shortcodes_dir') }}</li>
                    <li>{id} — {{ trans('dedicated_servers.d_shortcodes_id') }}</li>
                    <li>{uuid} — {{ trans('dedicated_servers.d_shortcodes_uuid') }}</li>
                    <li>{uuid_short} — {{ trans('dedicated_servers.d_shortcodes_uuid_short') }}</li>
                    <li>{game} — {{ trans('dedicated_servers.d_shortcodes_game') }}</li>
                    <li>{user} —{{ trans('dedicated_servers.d_shortcodes_user') }}</li>
                  </ul>
                </div>
                <div class="md:w-1/2 pr-4 pl-4">
                  <h5 class="text-xl font-bold dark:text-white">{{ trans('dedicated_servers.start_restart_shortcodes') }}</h5>
                  <ul class="ml-1 space-y-1 list-disc list-inside">
                    <li>{command} — {{ trans('dedicated_servers.d_shortcodes_start_command') }}</li>
                  </ul>

                  <h5 class="text-xl font-bold dark:text-white">{{ trans('dedicated_servers.console_command_shortcodes') }}</h5>
                  <ul class="ml-1 space-y-1 list-disc list-inside">
                    <li>{command} — {{ trans('dedicated_servers.d_shortcodes_console_command') }}</li>
                  </ul>
                </div>
              </div>
            </template>
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </n-form>

    <slot name="button">
      <GButton class="mt-2" color="green" v-on:click="onClickSend">
        <i class="fa-regular fa-floppy-disk mr-0.5"></i>
        <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
      </GButton>
    </slot>
  </div>
</template>

<script setup>
import {computed, ref, onMounted, h} from "vue"
import {allOfValidator, requiredValidator, stringLengthValidator} from "../../../parts/validators";
import {trans} from "../../../i18n/i18n";
import {
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NTabs,
  NTabPane,
  NSwitch,
  NSelect,
  NUpload,
} from "naive-ui"
import GButton from "../../../components/GButton.vue";

const osOptions = [
  {label: 'Linux', value: 'linux'},
  {label: 'Windows', value: 'windows'},
  {label: 'MacOS', value: 'macos', disabled: true},
]

const renderLabel = (option) => {
  switch (option.label) {
    case 'Linux':
      return [
          h('i', {class: 'fa-brands fa-linux'}, []),
          h('span', {class: 'ml-2'}, [option.label]),
      ]
    case 'Windows':
      return [
        h('i', {class: 'fa-brands fa-windows'}, []),
        h('span', {class: 'ml-2'}, [option.label]),
      ]
    case 'MacOS':
      return [
        h('i', {class: 'fa-brands fa-apple'}, []),
        h('span', {class: 'ml-2'}, [option.label]),
      ]
  }

  return option.label
}

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
  clientCertificateOptions: {
    type: Array,
    required: true,
  }
})

const formRef = ref({})
const form = defineModel({})

const rules = {
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
}

const handleChangeServerCertificate = (data) => {
  if (data.fileList.length > 0) {
    form.value.serverCertificateFile = data.fileList[0].file
  } else {
    form.value.serverCertificateFile = null
  }
}

const emits = defineEmits(['send'])

const onClickSend = () => {
  formRef.value.validate().then(() => {
    emits("send")
  })
}
</script>