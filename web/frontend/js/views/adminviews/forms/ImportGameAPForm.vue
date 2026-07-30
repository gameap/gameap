<template>
  <div>
    <p class="mb-4 text-stone-600 dark:text-stone-400">
      {{ trans('games.import_gameap_description') }}
    </p>

    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
    >
      <n-form-item :label="trans('games.gameap_yaml_file')">
        <n-upload
            accept=".yaml,.yml"
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
                .yaml, .yml
              </p>
            </div>
          </n-upload-dragger>
        </n-upload>
      </n-form-item>

      <div v-if="errorMessage" class="mb-4 p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 rounded">
        {{ errorMessage }}
      </div>

      <div v-if="yamlPreview" class="mb-4 p-4 bg-stone-100 dark:bg-stone-800 rounded">
        <div>
          <h4 class="font-semibold mb-2">{{ yamlPreview.gameName }}</h4>
          <p class="text-sm text-stone-600 dark:text-stone-400">
            {{ trans('labels.code') }}: {{ yamlPreview.gameCode }}
          </p>
          <p class="text-sm text-stone-600 dark:text-stone-400">
            {{ trans('labels.engine') }}: {{ yamlPreview.engine }}
          </p>
          <p v-if="yamlPreview.modsCount > 0" class="text-sm text-stone-600 dark:text-stone-400 mt-1">
            {{ trans('games.mods') }}: {{ yamlPreview.modsCount }}
          </p>
        </div>
      </div>

      <n-collapse v-if="yamlPreview" class="mb-4">
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
          :disabled="!yamlContent || importing"
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
import {
  NForm,
  NFormItem,
  NUpload,
  NUploadDragger,
  NCollapse,
  NCollapseItem,
  NInput,
} from "naive-ui"
import jsyaml from "js-yaml"

const formRef = ref({})
const errorMessage = ref('')
const yamlPreview = ref(null)
const yamlContent = ref(null)
const importing = ref(false)
const overrideName = ref('')
const overrideCode = ref('')

const emits = defineEmits(['import'])

const onFileChange = ({ file }) => {
  errorMessage.value = ''
  yamlPreview.value = null
  yamlContent.value = null

  if (!file || file.status === 'removed') {
    return
  }

  if (!file.file) {
    return
  }

  if (!file.name.endsWith('.yaml') && !file.name.endsWith('.yml')) {
    errorMessage.value = trans('games.invalid_file_type_yaml')
    return
  }

  const reader = new FileReader()
  reader.onload = (e) => {
    try {
      const parsed = jsyaml.load(e.target.result)

      if (!parsed.schema_version) {
        errorMessage.value = trans('games.gameap_yaml_missing_schema_version')
        return
      }

      if (parsed.schema_version !== '1.0') {
        errorMessage.value = trans('games.gameap_yaml_unsupported_schema_version')
        return
      }

      if (!parsed.game || !parsed.game.code) {
        errorMessage.value = trans('games.gameap_yaml_missing_game_code')
        return
      }

      if (!parsed.game.name) {
        errorMessage.value = trans('games.gameap_yaml_missing_game_name')
        return
      }

      if (!parsed.game.engine) {
        errorMessage.value = trans('games.gameap_yaml_missing_game_engine')
        return
      }

      yamlPreview.value = {
        gameName: parsed.game.name,
        gameCode: parsed.game.code,
        engine: parsed.game.engine,
        modsCount: parsed.mods ? parsed.mods.length : 0,
      }
      yamlContent.value = e.target.result
    } catch {
      errorMessage.value = trans('games.invalid_yaml_format')
    }
  }
  reader.onerror = () => {
    errorMessage.value = trans('games.file_read_error')
  }
  reader.readAsText(file.file)
}

const onClickImport = () => {
  if (!yamlContent.value) {
    return
  }

  importing.value = true
  emits('import', {
    content: yamlContent.value,
    name: overrideName.value || null,
    code: overrideCode.value || null,
  })
}

const resetForm = () => {
  errorMessage.value = ''
  yamlPreview.value = null
  yamlContent.value = null
  importing.value = false
  overrideName.value = ''
  overrideCode.value = ''
}

defineExpose({
  resetForm,
  setImporting: (value) => { importing.value = value },
})
</script>
