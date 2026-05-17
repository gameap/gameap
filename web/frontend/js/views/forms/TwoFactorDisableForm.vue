<template>
  <div :class="$attrs.class">
    <p class="mb-4 text-sm text-stone-500 dark:text-stone-300">
      {{ trans('two_factor.disable_desc') }}
    </p>

    <n-form
        ref="formRef"
        label-placement="top"
        :model="form"
        :rules="rules"
        @submit.prevent="onSubmit"
    >
      <n-form-item :label="trans('two_factor.password')" path="password">
        <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :input-props="{ autocomplete: 'current-password' }"
            data-testid="twofactor-disable-password"
        />
      </n-form-item>

      <n-form-item :label="trans('two_factor.code')" path="code">
        <n-input
            v-model:value="form.code"
            type="text"
            :placeholder="trans('two_factor.code_placeholder')"
            :input-props="{ autocomplete: 'one-time-code', inputmode: 'numeric' }"
            data-testid="twofactor-disable-code"
            @keyup.enter="onSubmit"
        />
      </n-form-item>
    </n-form>

    <div class="flex justify-end mt-4">
      <GButton color="red" data-testid="twofactor-disable-submit" v-on:click="onSubmit">
        <GIcon name="delete" class="mr-0.5" />
        <span class="inline">{{ trans('two_factor.disable') }}</span>
      </GButton>
    </div>
  </div>
</template>

<script setup>
import { ref } from "vue"
import { GIcon } from "@gameap/ui"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import { NForm, NFormItem, NInput } from "naive-ui"
import { requiredValidator } from "@/parts/validators"

const formRef = ref({})
const form = ref({
  password: '',
  code: '',
})

const rules = {
  password: {
    required: true,
    validator: requiredValidator(trans('two_factor.password')),
    trigger: ['blur', 'input'],
  },
  code: {
    required: true,
    validator: requiredValidator(trans('two_factor.code')),
    trigger: ['blur', 'input'],
  },
}

const emits = defineEmits(['disable'])

const onSubmit = () => {
  formRef.value.validate().then(() => {
    emits('disable', {
      password: form.value.password,
      code: form.value.code.trim(),
    })
  }).catch(() => {})
}
</script>
