<template>
  <div :class="$attrs.class">
    <p class="mb-4 text-sm text-stone-500 dark:text-stone-300">
      {{ trans('two_factor.verify_desc') }}
    </p>

    <n-form
        ref="formRef"
        label-placement="top"
        :model="form"
        :rules="rules"
        @submit.prevent="onSubmit"
    >
      <n-form-item :label="trans('two_factor.code')" path="code">
        <n-input
            v-model:value="form.code"
            type="text"
            autofocus
            :placeholder="trans('two_factor.code_placeholder')"
            :input-props="{ autocomplete: 'one-time-code', inputmode: 'numeric' }"
            data-testid="twofactor-verify-code"
            @keyup.enter="onSubmit"
        />
      </n-form-item>
    </n-form>

    <div class="flex justify-end mt-4">
      <GButton color="green" :loading="loading" data-testid="twofactor-verify-submit" v-on:click="onSubmit">
        <GIcon v-if="!loading" name="sign-in" class="mr-0.5" />
        <span class="inline">{{ loading ? trans('main.wait') : trans('two_factor.verify') }}</span>
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

const props = defineProps({
  loading: {
    type: Boolean,
    default: false,
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

const emits = defineEmits(['verify'])

const onSubmit = () => {
  if (props.loading) {
    return
  }

  formRef.value.validate().then(() => {
    emits('verify', form.value.code.trim())
  }).catch(() => {})
}
</script>
