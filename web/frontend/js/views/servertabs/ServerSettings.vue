<template>
  <GEmpty v-if="!settings || settings.length === 0" />
  <div v-else class="space-y-4">
    <div
      v-for="setting in settings"
      :key="setting.name"
      class="grid grid-cols-1 sm:grid-cols-[1fr_2fr] gap-2 sm:gap-4 sm:items-center"
    >
      <label class="text-sm sm:text-right truncate" :title="setting.label">
        {{ setting.label }}
      </label>
      <div>
        <GSwitch
          v-if="setting.type === 'bool'"
          v-model:value="settingsForm[setting.name]"
        />
        <n-input
          v-else-if="setting.type === 'string'"
          v-model:value="settingsForm[setting.name]"
          type="text"
        />
      </div>
    </div>

    <GFixedBottomBar>
      <GButton color="green" v-on:click="saveSettings()">
        <GIcon name="save" />
        <span class="inline">{{ trans('main.save') }}</span>
      </GButton>
    </GFixedBottomBar>
  </div>
</template>

<script setup>
import {trans} from "@/i18n/i18n"
import {useServerStore} from "@/store/server"
import {onMounted, ref} from "vue"
import {storeToRefs} from "pinia"
import { NInput } from "naive-ui"
import { GIcon, GEmpty, GSwitch } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import GFixedBottomBar from '@/components/GFixedBottomBar.vue'
import {errorNotification, notification} from "@/parts/dialogs";

const serverStore = useServerStore()

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