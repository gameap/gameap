<template>
  <div>
    <p class="mb-4 text-stone-600 dark:text-stone-400">
      {{ trans('games.import_pelican_egg_description') }}
    </p>

    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
    >
      <n-form-item :label="trans('games.pelican_egg_file')">
        <n-upload
            accept=".json,.yaml,.yml"
            :max="1"
            :default-upload="false"
            @change="onFileChange"
        >
          <n-upload-dragger>
            <div class="flex flex-col items-center gap-2 py-4">
              <GIcon name="upload" class="text-4xl text-stone-400" />
              <p class="text-stone-600 dark:text-stone-400">
                {{ trans('main.drag_file_here') }}
              </p>
              <p class="text-sm text-stone-500 dark:text-stone-500">
                {{ trans('main.or_click_to_select') }}
              </p>
              <p class="text-xs text-stone-400 dark:text-stone-600">
                .json, .yaml, .yml
              </p>
            </div>
          </n-upload-dragger>
        </n-upload>
      </n-form-item>

      <div v-if="errorMessage" class="mb-4 p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 rounded">
        {{ errorMessage }}
      </div>

      <div v-if="eggPreview" class="mb-4 p-4 bg-stone-100 dark:bg-stone-800 rounded">
        <div class="flex gap-4">
          <img
              v-if="eggPreview.image"
              :src="eggPreview.image"
              :alt="eggPreview.name"
              class="w-16 h-16 object-contain rounded"
          />
          <div>
            <h4 class="font-semibold mb-2">{{ eggPreview.name }}</h4>
            <p v-if="eggPreview.author" class="text-sm text-stone-600 dark:text-stone-400">
              {{ trans('games.author') }}: {{ eggPreview.author }}
            </p>
            <p v-if="eggPreview.description" class="text-sm text-stone-600 dark:text-stone-400 mt-1">
              {{ eggPreview.description }}
            </p>
            <p class="text-xs text-stone-500 dark:text-stone-500 mt-2">
              {{ trans('games.format') }}: {{ fileFormat.toUpperCase() }}
            </p>
          </div>
        </div>
      </div>

      <n-collapse v-if="eggPreview" class="mb-4">
        <n-collapse-item :title="trans('games.override_settings')" name="override">
          <n-form-item :label="trans('labels.name')">
            <n-input v-model:value="overrideName" :placeholder="trans('games.override_name_placeholder')" />
          </n-form-item>
          <n-form-item :label="trans('labels.code')">
            <n-input v-model:value="overrideCode" :placeholder="trans('games.override_code_placeholder')" />
          </n-form-item>
        </n-collapse-item>
      </n-collapse>
    </n-form>

    <GFixedBottomBar>
      <GButton
          color="blue"
          :disabled="!eggPreview || importing"
          :loading="importing"
          v-on:click="onClickImport"
      >
        <GIcon name="download" />
        <span class="inline">{{ trans('games.import') }}</span>
      </GButton>
    </GFixedBottomBar>
  </div>
</template>

<script setup>
import { GIcon } from "@gameap/ui"
import { ref } from "vue"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import GFixedBottomBar from "@/components/GFixedBottomBar.vue"
import yaml from "js-yaml"
import {
  NForm,
  NFormItem,
  NUpload,
  NUploadDragger,
  NCollapse,
  NCollapseItem,
  NInput,
} from "naive-ui"

const formRef = ref({})
const errorMessage = ref('')
const eggPreview = ref(null)
const eggContent = ref(null)
const fileFormat = ref('')
const importing = ref(false)
const overrideName = ref('')
const overrideCode = ref('')

const emits = defineEmits(['import'])

const detectFormat = (content) => {
  const trimmed = content.trim()
  if (trimmed.startsWith('{')) {
    return 'json'
  }
  return 'yaml'
}

const parseEggContent = (content, format) => {
  if (format === 'json') {
    return JSON.parse(content)
  }
  return yaml.load(content)
}

const onFileChange = ({ file }) => {
  errorMessage.value = ''
  eggPreview.value = null
  eggContent.value = null
  fileFormat.value = ''

  if (!file || file.status === 'removed') {
    return
  }

  if (!file.file) {
    return
  }

  const fileName = file.name.toLowerCase()
  if (!fileName.endsWith('.json') && !fileName.endsWith('.yaml') && !fileName.endsWith('.yml')) {
    errorMessage.value = trans('games.invalid_file_type_json_yaml')
    return
  }

  const reader = new FileReader()
  reader.onload = (e) => {
    try {
      const content = e.target.result
      const format = detectFormat(content)
      const parsed = parseEggContent(content, format)

      if (!parsed.name) {
        errorMessage.value = trans('games.pelican_egg_missing_name')
        return
      }

      if (!parsed.startup && !parsed.startup_commands?.Default) {
        errorMessage.value = trans('games.pelican_egg_missing_startup')
        return
      }

      eggPreview.value = {
        name: parsed.name,
        author: parsed.author || '',
        description: parsed.description || '',
        image: parsed.images?.icon?.url || parsed.image || '',
      }
      eggContent.value = content
      fileFormat.value = format
    } catch {
      errorMessage.value = trans('games.invalid_egg_format')
    }
  }
  reader.onerror = () => {
    errorMessage.value = trans('games.file_read_error')
  }
  reader.readAsText(file.file)
}

const onClickImport = () => {
  if (!eggContent.value) {
    return
  }

  importing.value = true
  emits('import', {
    content: eggContent.value,
    format: fileFormat.value,
    name: overrideName.value || null,
    code: overrideCode.value || null,
  })
}

const resetForm = () => {
  errorMessage.value = ''
  eggPreview.value = null
  eggContent.value = null
  fileFormat.value = ''
  importing.value = false
  overrideName.value = ''
  overrideCode.value = ''
}

defineExpose({
  resetForm,
  setImporting: (value) => { importing.value = value },
})
</script>
