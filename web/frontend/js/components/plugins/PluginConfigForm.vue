<template>
  <div class="plugin-config" data-testid="plugin-config">
    <div v-if="loadingConfig && !config" class="flex justify-center py-4">
      <Loading />
    </div>

    <div v-else-if="config">
      <div
        v-if="config.schema_error"
        class="mb-3 p-2 rounded-lg bg-warning-soft text-warning-soft-text text-sm break-words"
        data-testid="plugin-config-schema-error"
      >
        <span class="font-medium">{{ trans('plugins.config_schema_invalid') }}:</span>
        {{ config.schema_error }}
      </div>

      <p v-if="!hasSchema && !config.schema_error" class="text-xs text-stone-500 mb-3">
        {{ trans('plugins.config_no_schema_hint') }}
      </p>

      <div>
        <div v-if="properties.length > 0" class="grid grid-cols-1 md:grid-cols-2 gap-x-6">
          <n-form-item
            v-for="property in properties"
            :key="property.name"
            label-placement="top"
            :show-require-mark="false"
            :class="{ 'md:col-span-2': property.type === 'string' && !property.enum }"
            :validation-status="fieldErrors[property.name] ? 'error' : undefined"
            :feedback="fieldErrors[property.name] || undefined"
          >
            <template #label>
              <span class="flex items-center gap-1">
                <span class="font-medium">{{ property.title || property.name }}</span>
                <span v-if="property.required" class="text-danger" :title="trans('plugins.config_required')">*</span>
                <code v-if="property.title" class="text-[10px] text-stone-400">{{ property.name }}</code>
              </span>
            </template>

            <div class="w-full">
              <template v-if="property.secret">
                <n-input
                  v-model:value="secrets[property.name].value"
                  type="password"
                  show-password-on="click"
                  :disabled="secrets[property.name].clear"
                  :placeholder="secretPlaceholder(property)"
                  :data-testid="`plugin-config-field-${property.name}`"
                />
                <n-checkbox
                  v-if="secretIsSet(property.name)"
                  v-model:checked="secrets[property.name].clear"
                  class="mt-1"
                  :data-testid="`plugin-config-clear-${property.name}`"
                >
                  <span class="text-xs">{{ trans('plugins.config_secret_clear') }}</span>
                </n-checkbox>
              </template>

              <n-select
                v-else-if="property.enum"
                v-model:value="values[property.name]"
                :options="enumOptions(property)"
                :placeholder="defaultPlaceholder(property)"
                clearable
                :data-testid="`plugin-config-field-${property.name}`"
              />

              <n-switch
                v-else-if="property.type === 'boolean'"
                v-model:value="values[property.name]"
                :data-testid="`plugin-config-field-${property.name}`"
              />

              <n-input-number
                v-else-if="property.type === 'integer' || property.type === 'number'"
                v-model:value="values[property.name]"
                class="w-full"
                :min="property.minimum ?? undefined"
                :max="property.maximum ?? undefined"
                :precision="property.type === 'integer' ? 0 : undefined"
                :show-button="false"
                :placeholder="defaultPlaceholder(property)"
                :data-testid="`plugin-config-field-${property.name}`"
              />

              <n-input
                v-else
                v-model:value="values[property.name]"
                :placeholder="defaultPlaceholder(property)"
                :maxlength="property.max_length ?? undefined"
                :data-testid="`plugin-config-field-${property.name}`"
              />

              <p v-if="property.description" class="text-xs text-stone-500 mt-1">{{ property.description }}</p>
            </div>
          </n-form-item>
        </div>

        <div v-if="showExtraKeys" class="mt-2" data-testid="plugin-config-extra">
          <h4 class="text-sm font-medium mb-1">{{ trans('plugins.config_additional_keys') }}</h4>
          <p class="text-xs text-stone-500 mb-2">{{ trans('plugins.config_additional_keys_hint') }}</p>

          <div
            v-for="(row, index) in extraRows"
            :key="row.id"
            class="flex flex-col sm:flex-row gap-2 mb-2"
          >
            <div class="sm:w-1/3">
              <n-input
                v-model:value="row.key"
                :placeholder="trans('plugins.config_key')"
                :status="fieldErrors[row.key] ? 'error' : undefined"
                :data-testid="`plugin-config-extra-key-${index}`"
              />
              <p v-if="fieldErrors[row.key]" class="text-xs text-danger mt-1">{{ fieldErrors[row.key] }}</p>
            </div>
            <n-input
              v-model:value="row.value"
              class="flex-1"
              :placeholder="trans('plugins.config_value')"
              :data-testid="`plugin-config-extra-value-${index}`"
            />
            <GButton
              color="white"
              size="small"
              :title="trans('main.delete')"
              :data-testid="`plugin-config-extra-remove-${index}`"
              @click="removeExtraRow(index)"
            >
              <GIcon name="trash" />
            </GButton>
          </div>

          <GButton
            v-if="config.schema?.additional_properties !== false"
            color="white"
            size="small"
            data-testid="plugin-config-add-key"
            @click="addExtraRow"
          >
            <GIcon name="plus" class="mr-1" />
            {{ trans('plugins.config_add_key') }}
          </GButton>
        </div>

        <div
          v-if="formError"
          class="mt-3 p-2 rounded-lg bg-danger-soft text-danger-soft-text text-sm break-words"
          data-testid="plugin-config-error"
        >
          {{ formError }}
        </div>

        <div class="flex items-center justify-end gap-3 mt-3">
          <span v-if="!canReload" class="text-xs text-stone-500">{{ trans('plugins.config_no_reload_hint') }}</span>
          <GButton
            color="blue"
            :size="isSmallScreen ? 'small' : 'medium'"
            :disabled="saving"
            data-testid="plugin-config-save"
            @click="save"
          >
            <GIcon name="check" class="mr-1" />
            {{ trans('plugins.config_save') }}
          </GButton>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch, onMounted, onUnmounted } from 'vue'
import { trans } from '@/i18n/i18n'
import { GIcon, Loading } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { NFormItem, NInput, NInputNumber, NSwitch, NSelect, NCheckbox } from 'naive-ui'
import { usePluginStoreStore } from '@/store/pluginStore'
import { errorNotification, notification } from '@/parts/dialogs'

const props = defineProps({
  pluginId: {
    type: String,
    required: true
  },
  // The loaded row; a disabled or updating plugin is saved without a reload.
  loadedInfo: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['saved'])

const pluginStore = usePluginStoreStore()

const config = ref(null)
const loadingConfig = ref(false)
const saving = ref(false)
const formError = ref('')
const fieldErrors = ref({})

const values = reactive({})
const secrets = reactive({})
const extraRows = ref([])
let nextRowID = 1

const properties = computed(() => config.value?.schema?.properties ?? [])
const hasSchema = computed(() => properties.value.length > 0)
const schemaKeys = computed(() => new Set(properties.value.map(property => property.name)))

const canReload = computed(() => {
  const status = props.loadedInfo?.status

  return status !== 'disabled' && status !== 'updating'
})

// Free-form rows are offered for plugins without a schema and for stored
// keys the schema does not know; a closed schema hides them once none are
// stored.
const showExtraKeys = computed(() => {
  if (!config.value) return false
  if (config.value.schema?.additional_properties === false) {
    return extraRows.value.length > 0
  }

  return true
})

const isSmallScreen = ref(window.innerWidth < 768)

const handleResize = () => {
  isSmallScreen.value = window.innerWidth < 768
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  load()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

watch(() => props.pluginId, () => {
  load()
})

async function load() {
  loadingConfig.value = true
  formError.value = ''
  fieldErrors.value = {}

  try {
    applyView(await pluginStore.fetchPluginConfig(props.pluginId))
  } catch (error) {
    errorNotification(error)
  } finally {
    loadingConfig.value = false
  }
}

function applyView(view) {
  config.value = view

  for (const key of Object.keys(values)) {
    delete values[key]
  }
  for (const key of Object.keys(secrets)) {
    delete secrets[key]
  }

  const stored = view.values ?? {}
  const secretsSet = new Set(view.secrets_set ?? [])
  const known = new Set()

  for (const property of view.schema?.properties ?? []) {
    known.add(property.name)

    if (property.secret) {
      secrets[property.name] = { value: '', clear: false, set: secretsSet.has(property.name) }
      continue
    }

    if (property.name in stored) {
      values[property.name] = stored[property.name]
    } else if (property.type === 'boolean') {
      values[property.name] = property.default === true
    } else {
      values[property.name] = null
    }
  }

  extraRows.value = Object.entries(stored)
    .filter(([key]) => !known.has(key))
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({ id: nextRowID++, key, value: stringifyValue(value) }))
}

function stringifyValue(value) {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'object') return JSON.stringify(value)

  return String(value)
}

function secretIsSet(name) {
  return secrets[name]?.set === true
}

function secretPlaceholder(property) {
  return secretIsSet(property.name)
    ? trans('plugins.config_secret_set')
    : trans('plugins.config_secret_not_set')
}

function defaultPlaceholder(property) {
  if (property.default === undefined || property.default === null) return ''

  return trans('plugins.config_default') + ': ' + stringifyValue(property.default)
}

function enumOptions(property) {
  return (property.enum ?? []).map(value => ({
    label: stringifyValue(value),
    value,
  }))
}

function addExtraRow() {
  extraRows.value.push({ id: nextRowID++, key: '', value: '' })
}

function removeExtraRow(index) {
  extraRows.value.splice(index, 1)
}

// Blank inputs are left out so the schema default applies; secrets follow
// the keep / clear / replace contract of the API.
function collectValues() {
  const collected = {}

  for (const property of properties.value) {
    if (property.secret) {
      const state = secrets[property.name]
      if (state.clear) {
        collected[property.name] = ''
      } else if (state.value !== '') {
        collected[property.name] = state.value
      }
      continue
    }

    const value = values[property.name]
    if (value === null || value === undefined || value === '') {
      continue
    }

    collected[property.name] = value
  }

  for (const row of extraRows.value) {
    const key = row.key.trim()
    if (key === '' || schemaKeys.value.has(key)) {
      continue
    }

    collected[key] = row.value
  }

  return collected
}

async function save() {
  if (saving.value) return

  saving.value = true
  formError.value = ''
  fieldErrors.value = {}

  try {
    const result = await pluginStore.updatePluginConfig(props.pluginId, collectValues())
    applyView(result)
    notifySaved(result)
    emit('saved', result)
  } catch (error) {
    const data = error?.response?.data
    if (error?.response?.status === 422 && data?.errors && typeof data.errors === 'object') {
      fieldErrors.value = { ...data.errors }
      formError.value = data.title ? trans(data.title) : (data.message || '')
    } else {
      errorNotification(error)
    }
  } finally {
    saving.value = false
  }
}

function notifySaved(result) {
  if (result.reload_error) {
    notification({
      title: trans('plugins.reload_failed_title'),
      content: trans('plugins.config_saved_reload_failed') + '\n' + result.reload_error,
      type: 'error'
    })

    return
  }

  notification({
    content: result.reloaded
      ? trans('plugins.config_saved_reloaded')
      : trans('plugins.config_saved_not_reloaded'),
    type: 'success'
  })
}
</script>
