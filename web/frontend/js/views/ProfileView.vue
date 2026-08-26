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
        <td>{{ user.roles?.join(', ')  }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('profile.language') }}:</strong></td>
        <td>{{ currentLanguageLabel }}</td>
      </tr>
      </tbody>
    </GTable>
  </GCard>

  <GCard :title="trans('two_factor.title')" class="mb-3">
    <p class="mb-3 text-sm text-stone-500 dark:text-stone-300">
      {{ trans('two_factor.description') }}
    </p>
    <div class="flex items-center justify-between flex-wrap gap-3">
      <div class="flex items-center gap-2">
        <strong>{{ trans('two_factor.status') }}:</strong>
        <GStatusBadge
            :status="is2FAEnabled ? 'success' : 'canceled'"
            :text="is2FAEnabled ? trans('two_factor.enabled') : trans('two_factor.disabled')"
            data-testid="twofactor-status"
        />
      </div>
      <div class="flex flex-wrap gap-2">
        <GButton
            v-if="!is2FAEnabled"
            color="green"
            data-testid="twofactor-enable-button"
            v-on:click="onEnable"
        >
          <GIcon name="lock" class="mr-1" />
          <span>{{ trans('two_factor.enable') }}</span>
        </GButton>
        <template v-else>
          <GButton
              color="white"
              data-testid="twofactor-regen-button"
              v-on:click="onOpenRegen"
          >
            <GIcon name="refresh" class="mr-1" />
            <span>{{ trans('two_factor.regenerate_recovery_codes') }}</span>
          </GButton>
          <GButton
              color="red"
              data-testid="twofactor-disable-button"
              v-on:click="disableModalEnabled = true"
          >
            <GIcon name="delete" class="mr-1" />
            <span>{{ trans('two_factor.disable') }}</span>
          </GButton>
        </template>
      </div>
    </div>
  </GCard>

  <GModal
      v-model:show="updateProfileModalEnabled"
      :title="trans('profile.edit')"
      style="width: 600px"
  >
    <UpdateProfileForm v-model="updateProfileModel" v-on:update="onUpdate" />
  </GModal>

  <GModal
      v-model:show="setupModalEnabled"
      :title="trans('two_factor.setup_title')"
      style="width: 520px"
  >
    <TwoFactorSetupForm
        :secret="twoFASetup.secret"
        :otpauth-uri="twoFASetup.otpauth_uri"
        :recovery-codes="twoFARecoveryCodes"
        @confirm="onConfirm"
        @done="onSetupDone"
    />
  </GModal>

  <GModal
      v-model:show="disableModalEnabled"
      :title="trans('two_factor.disable_title')"
      style="width: 520px"
  >
    <TwoFactorDisableForm @disable="onDisable" />
  </GModal>

  <GModal
      v-model:show="regenModalEnabled"
      :title="trans('two_factor.regenerate_recovery_codes')"
      style="width: 520px"
  >
    <TwoFactorSetupForm
        v-if="twoFARecoveryCodes"
        :recovery-codes="twoFARecoveryCodes"
        @done="onRegenDone"
    />
    <div v-else>
      <p class="mb-4 text-sm text-stone-500 dark:text-stone-300">
        {{ trans('two_factor.regenerate_desc') }}
      </p>
      <n-form ref="regenFormRef" label-placement="top" :model="regenForm" :rules="regenRules">
        <n-form-item :label="trans('two_factor.password')" path="password">
          <n-input
              v-model:value="regenForm.password"
              type="password"
              show-password-on="click"
              :input-props="{ autocomplete: 'current-password' }"
              data-testid="twofactor-regen-password"
          />
        </n-form-item>
      </n-form>
      <div class="flex justify-end mt-4">
        <GButton color="green" data-testid="twofactor-regen-submit" v-on:click="onRegen">
          <GIcon name="refresh" class="mr-0.5" />
          <span>{{ trans('two_factor.regenerate_recovery_codes') }}</span>
        </GButton>
      </div>
    </div>
  </GModal>
</template>

<script setup>
import { GBreadcrumbs, GIcon, GModal, GCard, GTable, GStatusBadge } from "@gameap/ui"
import {computed, ref} from "vue"
import { NForm, NFormItem, NInput } from "naive-ui"
import {trans, getCurrentLanguage, changeLanguage, getAvailableLanguages} from "@/i18n/i18n"
import UpdateProfileForm from "./forms/UpdateProfileForm.vue";
import TwoFactorSetupForm from "./forms/TwoFactorSetupForm.vue";
import TwoFactorDisableForm from "./forms/TwoFactorDisableForm.vue";
import {useAuthStore} from "@/store/auth";
import {useUISettingsStore} from "@/store/uiSettings";
import GButton from "../components/GButton.vue";
import {errorNotification, notification} from "@/parts/dialogs";
import { requiredValidator } from "@/parts/validators";

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()

const currentLanguageLabel = computed(() => {
  const lang = getCurrentLanguage()
  const language = getAvailableLanguages().find((l) => l.code === lang)

  return language?.native_name || language?.name || lang
})

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
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

// --- Two-factor authentication ---

const is2FAEnabled = computed(() => authStore.is2FAEnabled)

const setupModalEnabled = ref(false)
const disableModalEnabled = ref(false)
const regenModalEnabled = ref(false)
const twoFASetup = ref({ secret: '', otpauth_uri: '' })
const twoFARecoveryCodes = ref(null)

const regenFormRef = ref({})
const regenForm = ref({ password: '' })
const regenRules = {
  password: {
    required: true,
    validator: requiredValidator(trans('two_factor.password')),
    trigger: ['blur', 'input'],
  },
}

const onEnable = () => {
  authStore.setupTwoFactor().then((data) => {
    twoFASetup.value = data
    twoFARecoveryCodes.value = null
    setupModalEnabled.value = true
  }).catch(errorNotification)
}

const onConfirm = (code) => {
  authStore.confirmTwoFactor(code).then((data) => {
    twoFARecoveryCodes.value = data.recovery_codes
  }).catch(errorNotification)
}

const onSetupDone = () => {
  setupModalEnabled.value = false
  twoFARecoveryCodes.value = null
  authStore.fetchProfile()
  notification({
    content: trans('two_factor.enable_success'),
    type: "success",
  })
}

const onDisable = ({ password, code }) => {
  authStore.disableTwoFactor(password, code).then(() => {
    disableModalEnabled.value = false
    authStore.fetchProfile()
    notification({
      content: trans('two_factor.disable_success'),
      type: "success",
    })
  }).catch(errorNotification)
}

const onOpenRegen = () => {
  twoFARecoveryCodes.value = null
  regenForm.value.password = ''
  regenModalEnabled.value = true
}

const onRegen = () => {
  regenFormRef.value.validate().then(() => {
    authStore.regenerateRecoveryCodes(regenForm.value.password).then((data) => {
      twoFARecoveryCodes.value = data.recovery_codes
    }).catch(errorNotification)
  }).catch(() => {})
}

const onRegenDone = () => {
  regenModalEnabled.value = false
  twoFARecoveryCodes.value = null
  notification({
    content: trans('two_factor.recovery_regenerated'),
    type: "success",
  })
}

</script>