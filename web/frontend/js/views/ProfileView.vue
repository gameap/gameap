<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" data-testid="profile-edit-button" v-on:click="onClickUpdate()">
    <GIcon name="edit" class="mr-1" />
    <span>{{ trans('profile.edit')}}</span>
  </GButton>

  <GCard :title="trans('profile.profile')" class="mb-3">
    <GTable>
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
        <td data-testid="profile-name-value">{{ user.name }}</td>
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
    </GTable>
  </GCard>

  <GModal
      v-model:show="updateProfileModalEnabled"
      :title="trans('profile.edit')"
      style="width: 600px"
  >
    <UpdateProfileForm v-model="updateProfileModel" v-on:update="onUpdate" />
  </GModal>
</template>

<script setup>
import { GBreadcrumbs, GIcon, GModal, GCard, GTable } from "@gameap/ui"
import {computed, ref} from "vue"
import {trans, getCurrentLanguage, changeLanguage} from "@/i18n/i18n"
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