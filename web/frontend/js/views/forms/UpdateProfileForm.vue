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
            data-testid="profile-form-name"
        />
      </n-form-item>

      <n-form-item :label="trans('profile.language')" path="language">
        <n-select
            v-model:value="form.language"
            :options="languageOptions"
        />
      </n-form-item>

      <GDivider>
        {{ trans('profile.change_password') }}
      </GDivider>

      <n-form-item :label="trans('labels.current_password')" path="currentPassword">
        <div class="flex w-full">
          <div class="grid grid-cols-1 w-full">
            <n-input
                v-model:value="form.currentPassword"
                type="password"
                show-password-on="click"
                :placeholder="trans('labels.current_password')"
                :input-props="{ autocomplete: 'one-time-code' }"
                data-testid="profile-form-current-password"
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
                data-testid="profile-form-new-password"
            />

            <n-input
                v-model:value="form.passwordConfirmation"
                type="password"
                show-password-on="click"
                :input-props="{ autocomplete: 'one-time-code' }"
                :placeholder="trans('labels.password_confirmation')"
                data-testid="profile-form-password-confirmation"
            />
          </div>
        </div>
      </n-form-item>
    </n-form>

    <div class="flex justify-end mt-4">
      <GButton color="green" data-testid="profile-form-save" v-on:click="onClickUpdate">
        <GIcon name="save" class="mr-0.5" />
        <span class="inline">{{ trans('main.save') }}</span>
      </GButton>
    </div>
  </div>
</template>

<script setup>
import {ref, defineModel} from "vue"
import { GIcon, GDivider } from "@gameap/ui"
import {trans} from "@/i18n/i18n";
import GButton from "@/components/GButton.vue";
import {
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