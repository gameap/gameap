<template>
  <n-empty v-if="!settings || settings.length === 0"></n-empty>
  <n-form
      v-else
      label-placement="left"
      label-width="auto"
      ref="settingsRef"
      :model="settingsForm"
  >
      <n-form-item v-for="setting in settings" :label="setting.label">
        <n-switch
            v-if="setting.type === 'bool'"
            v-model:value="settingsForm[setting.name]"
        >
        </n-switch>
        <n-input
            v-if="setting.type === 'string'"
            v-model:value="settingsForm[setting.name]"
            type="text"
        />
      </n-form-item>

      <GButton color="green" v-on:click="saveSettings()">
        <i class="fas fa-edit"></i>
        <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
      </GButton>
  </n-form>
</template>

<script setup>
import {trans} from "@/i18n/i18n"
import {useServerStore} from "@/store/server"
import {onMounted, ref} from "vue"
import {storeToRefs} from "pinia"
import {
  NForm,
  NFormItem,
  NInput,
  NEmpty,
  NSwitch,
} from "naive-ui"
import GButton from '@/components/GButton.vue'
import {errorNotification, notification} from "@/parts/dialogs";

const serverStore = useServerStore()

const settingsRef = ref({})
const settingsForm = ref({})

const {settings} = storeToRefs(serverStore)

onMounted(() => {
  serverStore.fetchSettings().
    catch((error) => {
      errorNotification(error)
    }).
    then(() => {
      for(const setting of settings.value) {
        if (setting.type === 'bool') {
          settingsForm.value[setting.name] = (
              setting.value === true ||
              setting.value === 1 || setting.value === '1' ||
              setting.value === 'true' || setting.value === 'True' || setting.value === 'TRUE' ||
              setting.value === 'on' || setting.value === 'On' || setting.value === 'ON'
          )
        } else {
          settingsForm.value[setting.name] = setting.value
        }
      }
    })
});

function saveSettings() {
  let settings = []
  for (const [key, value] of Object.entries(settingsForm.value)) {
    settings.push({
      name: key,
      value: value
    })
  }

  serverStore.saveSettings(settings).
    then(() => {
      notification({
        content: trans('servers.settings_update_success_msg'),
        type: 'success',
      }, () => {
        fetchSettings()
      })
    }).catch((error) => {
      errorNotification(error)
    })
}

function fetchSettings() {
  serverStore.fetchSettings().catch((error) => {
    errorNotification(error)
  })
}
</script>