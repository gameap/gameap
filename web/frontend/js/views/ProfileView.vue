<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" v-on:click="onClickUpdate()">
    <i class="fa-solid fa-user-pen mr-1"></i>
    <span>{{ trans('profile.edit')}}</span>
  </GButton>

  <n-card
      :title="trans('profile.profile')"
      size="small"
      class="mb-3"
      header-class="g-card-header"
      :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
  >
    <n-table :bordered="false" :single-line="true">
      <tbody>
      <tr>
        <td><strong>{{ trans('users.login') }}:</strong></td>
        <td>{{ user.login }}</td>
      </tr>
      <tr>
        <td><strong>Email:</strong></td>
        <td>{{ user.email }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('users.name') }}:</strong></td>
        <td>{{ user.name }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('users.roles') }}:</strong></td>
        <td>{{ user.roles.join(', ')  }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('profile.language') }}:</strong></td>
        <td>{{ currentLanguageLabel }}</td>
      </tr>
      </tbody>
    </n-table>
  </n-card>

  <n-modal
      v-model:show="updateProfileModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('profile.edit')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <UpdateProfileForm v-model="updateProfileModel" v-on:update="onUpdate" />
  </n-modal>
</template>

<script setup>
import {computed, ref} from "vue"
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import {trans, getCurrentLanguage, changeLanguage} from "@/i18n/i18n"
import {
  NCard,
  NTable,
  NModal,
} from "naive-ui"
import UpdateProfileForm from "./forms/UpdateProfileForm.vue";
import {useAuthStore} from "@/store/auth";
import {useUISettingsStore} from "@/store/uiSettings";
import GButton from "../components/GButton.vue";
import {errorNotification, notification} from "@/parts/dialogs";

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()

const languageLabels = {
  'en': 'English',
  'ru': 'Русский',
}

const currentLanguageLabel = computed(() => {
  const lang = getCurrentLanguage()
  return languageLabels[lang] || lang
})

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'profile'}, 'text':trans('profile.profile')},
  ]
})

const user = computed(() => {
  return authStore.user
})

const onClickUpdate = () => {
  updateProfileModel.value = {
    name: user.value.name,
    language: getCurrentLanguage(),
  }
  updateProfileModalEnabled.value = true
}

const updateProfileModalEnabled = ref(false)
const updateProfileModel = ref({
  name: user.value.name,
  language: getCurrentLanguage(),
})

const onUpdate = () => {
  const currentLang = getCurrentLanguage()
  const newLang = updateProfileModel.value.language

  let profile = {
    name: updateProfileModel.value.name,
  }

  if (updateProfileModel.value.password) {
    profile.current_password = updateProfileModel.value.currentPassword
    profile.password = updateProfileModel.value.password
  }

  authStore.saveProfile(profile).then(() => {
    if (newLang && newLang !== currentLang) {
      uiSettingsStore.setLanguage(newLang)
      changeLanguage(newLang)

      notification({
        content: trans('profile.update_with_lang_success_msg'),
        type: "success",
      }, () => {
        window.location.reload()
      })
    } else {
      notification({
        content: trans('profile.update_success_msg'),
        type: "success",
      })
    }

    authStore.fetchProfile()

    updateProfileModalEnabled.value = false
  }).catch((error) => {
    errorNotification(error)
    updateProfileModalEnabled.value = false
  })
}

</script>