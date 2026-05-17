<template>
  <div :class="$attrs.class">
    <!-- Step 2: one-time recovery codes (after a successful confirm) -->
    <template v-if="recoveryCodes && recoveryCodes.length">
      <p class="mb-2 text-sm text-stone-700 dark:text-stone-200 font-medium">
        {{ trans('two_factor.recovery_codes_title') }}
      </p>
      <p class="mb-3 text-sm text-stone-500 dark:text-stone-300">
        {{ trans('two_factor.recovery_codes_desc') }}
      </p>

      <n-alert type="warning" :show-icon="true" class="mb-3">
        {{ trans('two_factor.recovery_codes_warn') }}
      </n-alert>

      <div
          class="grid grid-cols-2 gap-2 font-mono text-sm bg-stone-100 dark:bg-stone-900 rounded p-3 mb-4"
          data-testid="twofactor-recovery-codes"
      >
        <span v-for="rc in recoveryCodes" :key="rc" class="select-all">{{ rc }}</span>
      </div>

      <div class="flex justify-end">
        <GButton color="green" data-testid="twofactor-setup-done" v-on:click="$emit('done')">
          <GIcon name="check" class="mr-0.5" />
          <span class="inline">{{ trans('two_factor.done') }}</span>
        </GButton>
      </div>
    </template>

    <!-- Step 1: scan + confirm -->
    <template v-else>
      <p class="mb-3 text-sm text-stone-500 dark:text-stone-300">
        {{ trans('two_factor.setup_step_scan') }}
      </p>

      <div class="flex justify-center mb-4">
        <n-qr-code :value="otpauthUri" :size="180" error-correction-level="M" />
      </div>

      <p class="mb-1 text-sm text-stone-500 dark:text-stone-300">
        {{ trans('two_factor.setup_step_manual') }}
      </p>
      <p
          class="mb-4 font-mono text-sm break-all select-all bg-stone-100 dark:bg-stone-900 rounded p-2"
          data-testid="twofactor-setup-secret"
      >
        {{ secret }}
      </p>

      <n-form
          ref="formRef"
          label-placement="top"
          :model="form"
          :rules="rules"
          @submit.prevent="onConfirm"
      >
        <n-form-item :label="trans('two_factor.setup_step_confirm')" path="code">
          <n-input
              v-model:value="form.code"
              type="text"
              :placeholder="trans('two_factor.code_placeholder')"
              :input-props="{ autocomplete: 'one-time-code', inputmode: 'numeric' }"
              data-testid="twofactor-setup-code"
              @keyup.enter="onConfirm"
          />
        </n-form-item>
      </n-form>

      <div class="flex justify-end mt-2">
        <GButton color="green" data-testid="twofactor-setup-confirm" v-on:click="onConfirm">
          <GIcon name="check" class="mr-0.5" />
          <span class="inline">{{ trans('two_factor.confirm') }}</span>
        </GButton>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref } from "vue"
import { GIcon } from "@gameap/ui"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import { NForm, NFormItem, NInput, NQrCode, NAlert } from "naive-ui"
import { requiredValidator } from "@/parts/validators"

defineProps({
  secret: {
    type: String,
    default: '',
  },
  otpauthUri: {
    type: String,
    default: '',
  },
  recoveryCodes: {
    type: Array,
    default: null,
  },
})

const formRef = ref({})
const form = ref({
  code: '',
})

const rules = {
  code: {
    required: true,
    validator: requiredValidator(trans('two_factor.code')),
    trigger: ['blur', 'input'],
  },
}

const emits = defineEmits(['confirm', 'done'])

const onConfirm = () => {
  formRef.value.validate().then(() => {
    emits('confirm', form.value.code.trim())
  }).catch(() => {})
}
</script>
