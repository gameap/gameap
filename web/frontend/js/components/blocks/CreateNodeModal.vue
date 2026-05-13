<template>
  <n-modal class="create-node-modal" v-model:show="showModal">
    <n-card
        :title="trans('dedicated_servers.autosetup_title')"
        style="max-width: 800px;min-height: 500px"
        :bordered="false"
        size="huge"
        role="dialog"
        aria-modal="true"
    >
      <template #header-extra>
        <button type="button" class="btn-close" aria-label="Close" v-on:click="showModal = false">
          <GIcon name="close" />
        </button>
      </template>

      <n-tabs v-model:value="activeTab" type="line" class="flex justify-between mb-2" animated>
        <n-tab-pane name="linux">
          <template #tab>
            <GIcon name="linux" class="mr-1" />Linux
          </template>

          <div class="md:w-full pr-4 pl-4 m-6">
            <template v-if="grpcEnabled">
              <p>{{ trans('dedicated_servers.autosetup_run_command') }}</p>

              <div class="curl-container">
                <code class="curl-link">{{ linuxCmd }}</code>
                <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('curl', linuxCmd)" :title="trans('main.copy')">
                  <Transition name="icon-fade" mode="out-in">
                    <GIcon v-if="copiedKey === 'curl'" name="check" class="copied-icon" key="check" />
                    <GIcon v-else name="copy" key="copy" />
                  </Transition>
                </button>
              </div>

              <p class="text-center">
                <small>{{ trans('dedicated_servers.autosetup_expire_msg') }}</small>
              </p>
            </template>

            <template v-else>
              <p>{{ trans('dedicated_servers.autosetup_gameapctl_hint') }}</p>

              <ul class="list-disc">
                <li class="copyable-item">
                  {{ trans('dedicated_servers.autosetup_host') }}: <code>{{ host }}</code>
                  <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('host', host)" :title="trans('main.copy')">
                    <Transition name="icon-fade" mode="out-in">
                      <GIcon v-if="copiedKey === 'host'" name="check" class="copied-icon" key="check" />
                      <GIcon v-else name="copy" key="copy" />
                    </Transition>
                  </button>
                </li>
                <li class="copyable-item">
                  {{ trans('dedicated_servers.autosetup_token') }}: <code>{{ token }}</code>
                  <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('token', token)" :title="trans('main.copy')">
                    <Transition name="icon-fade" mode="out-in">
                      <GIcon v-if="copiedKey === 'token'" name="check" class="copied-icon" key="check" />
                      <GIcon v-else name="copy" key="copy" />
                    </Transition>
                  </button>
                </li>
              </ul>

              <p>{{ trans('dedicated_servers.autosetup_run_command') }}</p>

              <div class="curl-container">
                <code class="curl-link">curl {{ linkWithConfig }} | bash --</code>
                <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('curl-legacy', curlCommand)" :title="trans('main.copy')">
                  <Transition name="icon-fade" mode="out-in">
                    <GIcon v-if="copiedKey === 'curl-legacy'" name="check" class="copied-icon" key="check" />
                    <GIcon v-else name="copy" key="copy" />
                  </Transition>
                </button>
              </div>

              <p class="text-center">
                <small>{{ trans('dedicated_servers.autosetup_expire_msg') }}</small>
              </p>
            </template>
          </div>

          <n-collapse>
            <n-collapse-item :title="trans('dedicated_servers.additional_settings')">
              <n-form-item :label="trans('dedicated_servers.process_manager')">
                <n-select v-model:value="daemonConfigProcessManager" :options="daemonConfigProcessManagerOptions" />
              </n-form-item>

              <n-form-item label="GitHub">
                <n-switch v-model:value="useGithubSource" />
                <span class="ml-2 text-sm text-gray-500">Install from GitHub source</span>
              </n-form-item>

              <n-form-item v-if="useGithubSource" label="Branch">
                <n-input v-model:value="branchName" placeholder="develop" />
              </n-form-item>
            </n-collapse-item>
          </n-collapse>
        </n-tab-pane>

        <n-tab-pane name="windows">
          <template #tab>
            <GIcon name="windows" class="mr-1" />Windows
          </template>

          <div class="md:w-full pr-4 pl-4 m-6">
            <template v-if="grpcEnabled">
              <ol class="list-decimal">
                <li>
                  {{ trans('dedicated_servers.autosetup_go_to') }}
                  <a target="_blank" href="https://github.com/gameap/gameapctl/releases">
                    {{ trans('dedicated_servers.autosetup_releases_page') }}
                  </a>
                </li>
                <li>{{ trans('dedicated_servers.autosetup_download_archive') }}</li>
                <li>{{ trans('dedicated_servers.autosetup_run_gameapctl') }}</li>
                <template v-if="!hasCustomParams">
                  <li>{{ trans('dedicated_servers.autosetup_click_install') }}</li>
                  <li>
                    {{ trans('dedicated_servers.autosetup_paste_connect_url') }}
                    <div class="curl-container">
                      <code class="curl-link">{{ connectURL }}</code>
                      <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('win-url', connectURL)" :title="trans('main.copy')">
                        <Transition name="icon-fade" mode="out-in">
                          <GIcon v-if="copiedKey === 'win-url'" name="check" class="copied-icon" key="check" />
                          <GIcon v-else name="copy" key="copy" />
                        </Transition>
                      </button>
                    </div>
                  </li>
                  <li>{{ trans('dedicated_servers.autosetup_push_install') }}</li>
                </template>
              </ol>

              <p>
                {{ hasCustomParams
                    ? trans('dedicated_servers.autosetup_run_command_windows')
                    : trans('dedicated_servers.autosetup_or_install_cli') }}
              </p>

              <div class="curl-container">
                <code class="curl-link">{{ windowsCmd }}</code>
                <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('win-cmd', windowsCmd)" :title="trans('main.copy')">
                  <Transition name="icon-fade" mode="out-in">
                    <GIcon v-if="copiedKey === 'win-cmd'" name="check" class="copied-icon" key="check" />
                    <GIcon v-else name="copy" key="copy" />
                  </Transition>
                </button>
              </div>

              <p class="text-center">
                <small>{{ trans('dedicated_servers.autosetup_expire_token_msg') }}</small>
              </p>
            </template>

            <template v-else>
              <p>{{ trans('dedicated_servers.autosetup_windows_only') }}</p>

              <ol class="list-decimal">
                <li>
                  {{ trans('dedicated_servers.autosetup_go_to') }}
                  <a target="_blank" href="https://github.com/gameap/gameapctl/releases">
                    {{ trans('dedicated_servers.autosetup_releases_page') }}
                  </a>
                </li>
                <li>{{ trans('dedicated_servers.autosetup_download_archive') }}</li>
                <li>{{ trans('dedicated_servers.autosetup_run_gameapctl') }}</li>
                <li>{{ trans('dedicated_servers.autosetup_click_install') }}</li>
                <li>
                  {{ trans('dedicated_servers.autosetup_fill_fields') }}
                  <ul class="list-disc ml-4">
                    <li class="copyable-item">
                      {{ trans('dedicated_servers.autosetup_host') }}: <code>{{ host }}</code>
                      <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('host-win', host)" :title="trans('main.copy')">
                        <Transition name="icon-fade" mode="out-in">
                          <GIcon v-if="copiedKey === 'host-win'" name="check" class="copied-icon" key="check" />
                          <GIcon v-else name="copy" key="copy" />
                        </Transition>
                      </button>
                    </li>
                    <li class="copyable-item">
                      {{ trans('dedicated_servers.autosetup_token') }}: <code>{{ token }}</code>
                      <button v-if="canCopy" class="copy-btn" @click="copyToClipboard('token-win', token)" :title="trans('main.copy')">
                        <Transition name="icon-fade" mode="out-in">
                          <GIcon v-if="copiedKey === 'token-win'" name="check" class="copied-icon" key="check" />
                          <GIcon v-else name="copy" key="copy" />
                        </Transition>
                      </button>
                    </li>
                  </ul>
                </li>
                <li>{{ trans('dedicated_servers.autosetup_push_install') }}</li>
              </ol>

              <p class="text-center">
                <small>{{ trans('dedicated_servers.autosetup_expire_token_msg') }}</small>
              </p>
            </template>
          </div>
        </n-tab-pane>
      </n-tabs>
    </n-card>
  </n-modal>
</template>

<script setup>
import {trans} from "@/i18n/i18n";
import {computed, ref, watch} from "vue";
import { GIcon } from '@gameap/ui';
import {useNodeListStore} from "@/store/nodeList";
import {storeToRefs} from "pinia"
import {NFormItem, NSwitch, NInput} from "naive-ui";
import {errorNotification} from "@/parts/dialogs";
import {copyToClipboard as copyText, isClipboardSupported} from "@/utils/clipboard";

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue'])

const nodeListStore = useNodeListStore()

const { autoSetupData } = storeToRefs(nodeListStore)

const grpcEnabled = computed(() => {
  return autoSetupData.value.grpc_enabled
})

const host = computed(() => {
  return autoSetupData.value.host
})

const token = computed(() => {
  return autoSetupData.value.token
})

const hasCustomParams = computed(() => {
  return (daemonConfigProcessManager.value && daemonConfigProcessManager.value !== 'auto')
      || useGithubSource.value
      || branchName.value
})

const buildQueryParams = () => {
  const params = new URLSearchParams()
  if (daemonConfigProcessManager.value && daemonConfigProcessManager.value !== 'auto') {
    params.set('config', `process_manager.name=${daemonConfigProcessManager.value}`)
  }
  if (useGithubSource.value) {
    params.set('github', 'true')
  }
  if (branchName.value) {
    params.set('branch', branchName.value)
  }
  return params.toString()
}

const buildLinuxCliArgs = () => {
  let args = ''
  if (daemonConfigProcessManager.value && daemonConfigProcessManager.value !== 'auto') {
    args += ` --config='process_manager.name=${daemonConfigProcessManager.value}'`
  }
  if (useGithubSource.value) {
    args += ' --github'
  }
  if (branchName.value) {
    args += ` --branch=${branchName.value}`
  }
  return args
}

const linuxCmd = computed(() => {
  const cmd = autoSetupData.value.linux_cmd || ''
  if (!hasCustomParams.value) {
    return cmd
  }
  return `${cmd}${buildLinuxCliArgs()}`
})

const windowsCmd = computed(() => {
  let cmd = autoSetupData.value.windows_cmd || ''
  if (daemonConfigProcessManager.value && daemonConfigProcessManager.value !== 'auto') {
    cmd += ` --config="process_manager.name=${daemonConfigProcessManager.value}"`
  }
  if (useGithubSource.value) {
    cmd += ' --github'
  }
  if (branchName.value) {
    cmd += ` --branch=${branchName.value}`
  }
  return cmd
})

const connectURL = computed(() => autoSetupData.value.connect_url || '')

const showModal = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
});

const activeTab = ref('linux')

const daemonConfigProcessManager = ref('auto');
const useGithubSource = ref(false);
const branchName = ref('');

const daemonConfigProcessManagerOptions = computed(() => {
  const linuxOptions = [
    { label: trans('dedicated_servers.pm_auto'), value: 'auto' },
    { label: trans('dedicated_servers.pm_simple'), value: 'simple' },
    { label: 'SystemD', value: 'systemd' },
    { label: 'Docker', value: 'docker' },
    { label: 'Podman', value: 'podman' },
    { label: 'Tmux', value: 'tmux' },
  ]
  const windowsOptions = [
    { label: trans('dedicated_servers.pm_auto'), value: 'auto' },
    { label: trans('dedicated_servers.pm_simple'), value: 'simple' },
    { label: 'WinSW', value: 'winsw' },
    { label: 'Shawl', value: 'shawl' },
  ]
  return activeTab.value === 'linux' ? linuxOptions : windowsOptions
})

watch(activeTab, () => {
  const availableValues = daemonConfigProcessManagerOptions.value.map(o => o.value)
  if (!availableValues.includes(daemonConfigProcessManager.value)) {
    daemonConfigProcessManager.value = 'auto'
  }
})

const linkWithConfig = computed(() => {
  const baseLink = autoSetupData.value.link
  if (!hasCustomParams.value) {
    return baseLink
  }
  const separator = baseLink.includes('?') ? '&' : '?'
  return `${baseLink}${separator}${buildQueryParams()}`
})

const curlCommand = computed(() => {
  return `curl '${linkWithConfig.value}' | bash --`
})

const copiedKey = ref(null)
let copyTimeout = null
const canCopy = isClipboardSupported()

const copyToClipboard = async (key, text) => {
  const ok = await copyText(text)
  if (!ok) {
    console.error('Failed to copy to clipboard')

    return
  }

  if (copyTimeout) {
    clearTimeout(copyTimeout)
  }

  copiedKey.value = key
  copyTimeout = setTimeout(() => {
    copiedKey.value = null
  }, 2000)
}

watch(() => props.modelValue, (visible) => {
  if (visible) {
    nodeListStore.fetchAutoSetupData().catch((error) => {
      errorNotification(error)
    })
  }
});

</script>

<style>
.create-node-modal {
  code {
    @apply text-rose-800 dark:text-rose-300 text-sm;
    word-wrap: break-word;
  }

  .curl-link {
    @apply bg-stone-50 dark:bg-stone-600 p-2 pr-10 my-1 rounded block;
    font-family: monospace;
    word-break: break-all;
    overflow-wrap: anywhere;
  }

  .curl-container {
    @apply relative;

    .copy-btn {
      @apply absolute right-2 top-1/2;
      transform: translateY(-50%);
    }
  }

  .copyable-item {
    @apply flex items-center gap-2 flex-wrap;
  }

  .copy-btn {
    @apply p-1 rounded hover:bg-stone-200 dark:hover:bg-stone-500 text-stone-500 dark:text-stone-300 transition-colors;
  }

  .copied-icon {
    @apply text-green-500;
  }

  .icon-fade-enter-active,
  .icon-fade-leave-active {
    transition: opacity 0.15s ease, transform 0.15s ease;
  }

  .icon-fade-enter-from {
    opacity: 0;
    transform: scale(0.8);
  }

  .icon-fade-leave-to {
    opacity: 0;
    transform: scale(0.8);
  }

  ul {
    @apply list-disc mb-2;
  }

  ol {
    @apply list-decimal mb-2;
  }

  li {
    @apply ml-10;
  }

  p {
    @apply my-1;
  }

  a {
    @apply font-medium text-blue-600 dark:text-blue-500 underline hover:no-underline;
  }
}
</style>
