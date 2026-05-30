<template>
  <n-modal
      :show="visible"
      preset="card"
      :title="title"
      style="width: min(94vw, 520px)"
      :bordered="false"
      :closable="!isHardFail"
      :mask-closable="false"
      :close-on-esc="!isHardFail"
      :auto-focus="false"
      data-testid="mfa-enforcement-modal"
      @update:show="onUpdateShow"
  >
    <TwoFactorSetupForm
        v-if="setupStarted"
        :secret="setup.secret"
        :otpauth-uri="setup.otpauth_uri"
        :recovery-codes="recoveryCodes"
        @confirm="onConfirm"
        @done="onDone"
    />

    <div v-else>
      <n-alert v-if="isHardFail" type="error" :show-icon="true" class="mb-3">
        {{ trans('mfa_enforcement.hard_fail_warning') }}
      </n-alert>

      <p class="mb-3 text-sm text-stone-500 dark:text-stone-300">
        {{ isHardFail ? trans('mfa_enforcement.hard_fail_desc') : trans('mfa_enforcement.desc') }}
      </p>

      <p
          v-if="!isHardFail && daysRemaining > 0"
          class="mb-4 text-sm text-stone-500 dark:text-stone-300"
      >
        {{ trans('mfa_enforcement.days_remaining', { days: daysRemaining }) }}
      </p>

      <div class="flex justify-end gap-2">
        <GButton
            v-if="!isHardFail"
            color="white"
            data-testid="mfa-enforcement-snooze"
            v-on:click="onSnooze"
        >
          <span>{{ trans('mfa_enforcement.remind_later') }}</span>
        </GButton>
        <GButton color="green" data-testid="mfa-enforcement-enable" v-on:click="onEnable">
          <GIcon name="lock" class="mr-1" />
          <span>{{ trans('mfa_enforcement.enable_now') }}</span>
        </GButton>
      </div>
    </div>
  </n-modal>
</template>

<script setup>
import { computed, ref } from "vue"
import { NModal, NAlert } from "naive-ui"
import { GIcon } from "@gameap/ui"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import TwoFactorSetupForm from "@/views/forms/TwoFactorSetupForm.vue"
import { useAuthStore } from "@/store/auth"
import { errorNotification, notification } from "@/parts/dialogs"

const authStore = useAuthStore()

const nudge = computed(() => authStore.mfaNudge)
const isHardFail = computed(() => !!nudge.value?.hard_fail)
const daysRemaining = computed(() => nudge.value?.days_remaining ?? 0)
const visible = computed(() => !!(nudge.value?.required && nudge.value?.show_now))

const title = computed(() =>
    isHardFail.value ? trans('mfa_enforcement.hard_fail_title') : trans('mfa_enforcement.title'),
)

const setupStarted = ref(false)
const setup = ref({ secret: '', otpauth_uri: '' })
const recoveryCodes = ref(null)

// A soft nudge can be dismissed (X / esc); treat that as "remind me later".
// Hard-fail disables closing, so this only fires for the soft case.
const onUpdateShow = (value) => {
  if (!value && !isHardFail.value && !setupStarted.value) {
    onSnooze()
  }
}

const onEnable = () => {
  authStore.setupTwoFactor().then((data) => {
    setup.value = data
    recoveryCodes.value = null
    setupStarted.value = true
  }).catch(errorNotification)
}

const onConfirm = (code) => {
  authStore.confirmTwoFactor(code).then((data) => {
    recoveryCodes.value = data.recovery_codes
  }).catch(errorNotification)
}

const onDone = () => {
  setupStarted.value = false
  recoveryCodes.value = null

  notification({ content: trans('two_factor.enable_success'), type: "success" })

  if (isHardFail.value) {
    // The enrollment-scoped token cannot be upgraded in place — re-login to
    // obtain a full session now that 2FA is enabled.
    authStore.logout().then(() => { window.location.href = '/' })

    return
  }

  authStore.fetchProfile()
}

const onSnooze = () => {
  authStore.snoozeMfaNudge().catch(errorNotification)
}
</script>
