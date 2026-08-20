<template>
  <GEmpty v-if="definitions.length === 0" />
  <n-form
    v-else
    ref="formRef"
    :model="formModel"
    label-placement="top"
    label-width="auto"
    data-testid="server-settings-form"
  >
    <VarFormItem
      v-for="definition in definitions"
      :key="definition.name"
      :definition="definition"
      :path="varPath(definition.name)"
      :server-error="serverErrors[definition.name] ?? null"
      :value="formModel.values[definition.name] ?? null"
      @update:value="setValue(definition, $event)"
    />

    <GFixedBottomBar>
      <GButton color="green" data-testid="server-settings-save" v-on:click="saveSettings()">
        <GIcon name="save" />
        <span class="inline">{{ trans('main.save') }}</span>
      </GButton>
    </GFixedBottomBar>
  </n-form>
</template>

<script setup>
import {trans} from "@/i18n/i18n"
import {useServerStore} from "@/store/server"
import {computed, onMounted, reactive, ref, watch} from "vue"
import {storeToRefs} from "pinia"
import { NForm } from "naive-ui"
import { GIcon, GEmpty } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import GFixedBottomBar from '@/components/GFixedBottomBar.vue'
import VarFormItem from '@/components/input/VarFormItem.vue'
import {coerceValue, normalizeVarDefinition, serializeValue} from '@/parts/gameModVars'
import {errorNotification, notification} from "@/parts/dialogs";

// The two panel-owned settings carry a hard-coded English label from the API.
const BUILT_IN_LABELS = {
  autostart: 'servers.autostart_setting',
  update_before_start: 'servers.update_before_start_setting',
}

const serverStore = useServerStore()
const {settings} = storeToRefs(serverStore)

const formRef = ref(null)
const formModel = reactive({values: {}})
const serverErrors = ref({})

const definitions = computed(() =>
    (settings.value || []).map((setting) => {
      const definition = normalizeVarDefinition(setting)

      const labelKey = BUILT_IN_LABELS[definition.name]
      if (labelKey) {
        definition.label = trans(labelKey)
      }

      return definition
    }),
)

// Rebuilt from the store rather than inside the fetch callback: that way a
// failed request leaves the form empty instead of half-populated, and the
// refresh after a save actually reaches the form.
watch(definitions, (list) => {
  const values = {}
  for (const definition of list) {
    values[definition.name] = coerceValue(definition, definition.default ?? null)

    const stored = (settings.value || []).find((setting) => setting.name === definition.name)
    if (stored) {
      values[definition.name] = coerceValue(definition, stored.value)
    }
  }

  formModel.values = values
  serverErrors.value = {}
}, {immediate: true})

onMounted(() => {
  fetchSettings()
})

// Names come out of the database, so the path is bracket-escaped rather than
// trusting dot notation.
function varPath(name) {
  return `values[${JSON.stringify(name)}]`
}

function setValue(definition, value) {
  formModel.values[definition.name] = value
  delete serverErrors.value[definition.name]
}

async function saveSettings() {
  serverErrors.value = {}

  try {
    await formRef.value.validate()
  } catch {
    notification({content: trans('servers.settings_check_form'), type: 'error'})

    return
  }

  const payload = definitions.value.map((definition) => ({
    name: definition.name,
    value: serializeValue(definition, formModel.values[definition.name]),
  }))

  try {
    await serverStore.saveSettings(payload)

    notification({
      content: trans('servers.settings_update_success_msg'),
      type: 'success',
    }, () => {
      fetchSettings()
    })
  } catch (error) {
    if (!applyServerErrors(error)) {
      errorNotification(error)
    }
  }
}

// Per-field highlighting needs the 422 body to carry an `errors` map; a plain
// message falls through to the generic error dialog.
function applyServerErrors(error) {
  if (error?.response?.status !== 422) {
    return false
  }

  const raw = error.response.data?.errors
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return false
  }

  serverErrors.value = Object.fromEntries(
      Object.entries(raw).map(([name, messages]) => [
        name,
        Array.isArray(messages) ? messages.join(' ') : String(messages),
      ]),
  )

  notification({content: trans('servers.settings_check_form'), type: 'error'})

  return true
}

function fetchSettings() {
  serverStore.fetchSettings().catch((error) => {
    errorNotification(error)
  })
}
</script>
