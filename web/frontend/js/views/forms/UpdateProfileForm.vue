<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <n-form-item :label="trans('labels.name')" path="name">
        <n-input
            v-model:value="form.name"
            type="text"
        />
      </n-form-item>

      <n-form-item :label="trans('profile.language')" path="language">
        <n-select
            v-model:value="form.language"
            :options="languageOptions"
        />
      </n-form-item>

      <n-divider>
        {{ trans('profile.change_password') }}
      </n-divider>

      <n-form-item :label="trans('labels.current_password')" path="currentPassword">
        <div class="flex w-full">
          <div class="grid grid-cols-1 w-full">
            <n-input
                v-model:value="form.currentPassword"
                type="password"
                show-password-on="click"
                :placeholder="trans('labels.current_password')"
                :input-props="{ autocomplete: 'one-time-code' }"
            />
          </div>
        </div>
      </n-form-item>

      <n-form-item :label="trans('labels.password')" path="password">
        <div class="flex w-full">
          <div class="grid grid-cols-2 gap-x-4 w-full">
            <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="trans('labels.new_password')"
                :input-props="{ autocomplete: 'one-time-code' }"
            />

            <n-input
                v-model:value="form.passwordConfirmation"
                type="password"
                show-password-on="click"
                :input-props="{ autocomplete: 'one-time-code' }"
                :placeholder="trans('labels.password_confirmation')"
            />
          </div>
        </div>
      </n-form-item>
    </n-form>

    <GButton color="green" v-on:click="onClickUpdate" class="mt-4">
      <i class="fa-regular fa-floppy-disk mr-0.5"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
    </GButton>
  </div>
</template>

<script setup>
import {ref, defineModel} from "vue"
import {trans} from "@/i18n/i18n";
import GButton from "@/components/GButton.vue";
import {
  NDivider,
  NForm,
  NFormItem,
  NInput,
  NSelect,
} from "naive-ui"
import {
  allOfValidator,
  requiredValidator,
  stringLengthValidator,
  sameWithValidator,
  stringMinLengthValidator,
  isArrayNotEmptyValidator, ifNotEmptyValidator,
} from "@/parts/validators"

const formRef = ref({})
const form = defineModel({
  name: '',
  language: '',
  currentPassword: '',
  password: '',
  passwordConfirmation: '',
})

const languageOptions = [
  {
    label: 'English',
    value: 'en',
  },
  {
    label: 'Русский',
    value: 'ru',
  },
]

const rules = {
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
  password: {
    validator: ifNotEmptyValidator(
        allOfValidator(
            stringMinLengthValidator(trans('labels.password'), 8),
            sameWithValidator(
                trans('labels.password'),
                trans('labels.password_confirmation'),
                () => form.value.passwordConfirmation,
            )
        ),
    ),
  },
}

const emits = defineEmits(['update'])

const onClickUpdate = () => {
  formRef.value.validate().then(() => {
    emits("update")
  })
}

</script>