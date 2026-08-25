<template>
  <GModal
    v-model:show="visible"
    :title="trans('plugins.upload_plugin')"
    style="width: 600px; max-width: 90vw;"
  >
    <n-spin :show="loading">
      <div v-if="!uploadResult">
        <n-upload
          :max="1"
          :multiple="false"
          :default-upload="false"
          accept=".wasm"
          @change="handleFileChange"
        >
          <n-upload-dragger>
            <div class="flex flex-col items-center py-4">
              <GIcon name="upload" class="text-4xl text-stone-400 mb-2" />
              <p class="text-sm">{{ trans('plugins.upload_hint') }}</p>
              <p class="text-xs text-stone-500 mt-1">.wasm, max 100MB</p>
            </div>
          </n-upload-dragger>
        </n-upload>

        <div class="flex justify-end gap-2 mt-4">
          <GButton color="gray" @click="close">{{ trans('main.close') }}</GButton>
          <GButton color="blue" :disabled="!selectedFile" @click="validatePlugin">
            {{ trans('plugins.validate') }}
          </GButton>
        </div>
      </div>

      <div v-else>
        <div class="flex items-center gap-3 mb-4">
          <GIcon name="puzzle-piece" class="text-4xl text-stone-400" />
          <div>
            <h3 class="text-lg font-bold">{{ uploadResult.name }}</h3>
            <span class="text-sm text-stone-500">v{{ uploadResult.version }}</span>
          </div>
          <GStatusBadge
            :color="uploadResult.is_valid ? 'green' : 'red'"
            :text="uploadResult.is_valid ? trans('plugins.valid') : trans('plugins.invalid')"
            class="ml-auto"
          />
        </div>

        <div v-if="uploadResult.description" class="mb-4 text-sm text-stone-600 dark:text-stone-400">
          {{ uploadResult.description }}
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4 p-3 bg-stone-100 dark:bg-stone-800 rounded-lg">
          <div v-if="uploadResult.author">
            <span class="text-xs text-stone-500">{{ trans('plugins.author') }}</span>
            <p class="text-sm font-medium">{{ uploadResult.author }}</p>
          </div>
          <div>
            <span class="text-xs text-stone-500">API Version</span>
            <p class="text-sm font-medium">{{ uploadResult.api_version || '-' }}</p>
          </div>
          <div>
            <span class="text-xs text-stone-500">{{ trans('plugins.features') }}</span>
            <p class="text-sm font-medium">
              {{ formatFeatures(uploadResult) }}
            </p>
          </div>
          <div>
            <span class="text-xs text-stone-500">Frontend</span>
            <p class="text-sm font-medium">
              {{ uploadResult.has_frontend_bundle ? bytesToHuman(uploadResult.frontend_bundle_size) : '-' }}
            </p>
          </div>
        </div>

        <div v-if="uploadResult.required_permissions?.length" class="mb-4" data-testid="dry-run-permissions">
          <span class="text-xs text-stone-500">{{ trans('plugins.required_permissions') }}</span>
          <div class="flex flex-wrap gap-1 mt-1">
            <GStatusBadge
              v-for="permission in uploadResult.required_permissions"
              :key="permission"
              color="blue"
              :text="permissionLabel(permission)"
            />
          </div>
        </div>

        <n-alert
          v-if="uploadResult.undeclared_permissions?.length"
          type="warning"
          :show-icon="true"
          class="mb-4"
          data-testid="dry-run-undeclared-permissions"
        >
          {{ trans('plugins.permissions_undeclared_warning') }}
          <span class="font-medium">{{ uploadResult.undeclared_permissions.map(permissionLabel).join(', ') }}</span>
        </n-alert>

        <n-alert
          v-if="uploadResult.errors?.length"
          type="error"
          :show-icon="true"
          class="mb-4"
          :title="trans('plugins.validation_errors')"
        >
          <ul class="list-disc list-inside text-sm">
            <li v-for="error in uploadResult.errors" :key="error">{{ error }}</li>
          </ul>
        </n-alert>

        <div class="flex justify-end gap-2 mt-4">
          <GButton color="gray" @click="resetUpload">{{ trans('main.back') }}</GButton>
          <GButton
            color="green"
            :disabled="!uploadResult.is_valid"
            @click="installPlugin"
          >
            <GIcon name="download" class="mr-1" />
            {{ trans('plugins.install') }}
          </GButton>
        </div>
      </div>
    </n-spin>
  </GModal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { trans } from '@/i18n/i18n'
import { GIcon, GModal, GStatusBadge } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { NAlert, NUpload, NUploadDragger, NSpin } from 'naive-ui'
import { usePluginStoreStore } from '@/store/pluginStore'
import { storeToRefs } from 'pinia'
import { errorNotification, notification } from '@/parts/dialogs'

const props = defineProps({
  show: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:show', 'installed'])

const pluginStore = usePluginStoreStore()
const { uploadResult, loading } = storeToRefs(pluginStore)

const selectedFile = ref(null)

const visible = computed({
  get: () => props.show,
  set: (val) => emit('update:show', val)
})

watch(visible, (val) => {
  if (!val) {
    resetUpload()
  }
})

function handleFileChange({ fileList }) {
  selectedFile.value = fileList.length > 0 ? fileList[0].file : null
}

async function validatePlugin() {
  if (!selectedFile.value) return

  try {
    await pluginStore.dryRunUpload(selectedFile.value)
  } catch (err) {
    errorNotification(err)
  }
}

async function installPlugin() {
  if (!selectedFile.value) return

  try {
    await pluginStore.installFromFile(selectedFile.value)
    notification({
      content: trans('plugins.install_from_file_success'),
      type: 'success'
    })
    emit('installed')
    close()
  } catch (err) {
    errorNotification(err)
  }
}

function resetUpload() {
  selectedFile.value = null
  pluginStore.clearUpload()
}

function close() {
  visible.value = false
}

function permissionLabel(permission) {
  return trans('plugins.permission_' + permission)
}

function formatFeatures(result) {
  const features = []
  if (result.http_routes?.length > 0) features.push('HTTP')
  if (result.server_abilities?.length > 0) features.push('Commands')
  if (result.has_frontend_bundle) features.push('Frontend')
  return features.length > 0 ? features.join(', ') : '-'
}

function bytesToHuman(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return `${bytes.toFixed(1)} ${units[i]}`
}
</script>
