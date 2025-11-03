<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <Loading v-if="loading"></Loading>
      <div class="mb-4" :class="loading ? 'hidden' : ''">
        <n-form-item :label="trans('labels.login')" path="login">
          <n-input
              v-model:value="form.login"
              type="text"
          />
        </n-form-item>

        <n-form-item :label="trans('labels.email')" path="email">
          <n-input
              v-model:value="form.email"
              type="text"
          />
        </n-form-item>

        <n-form-item :label="trans('labels.name')" path="name">
          <n-input
              v-model:value="form.name"
              type="text"
          />
        </n-form-item>

        <n-form-item :label="trans('labels.roles')" path="roles">
          <n-select
              v-model:value="form.roles"
              multiple
              :options="roles"
          />
        </n-form-item>

        <n-form-item :label="trans('labels.password')" path="password">
          <div class="flex grid-cols-2 gap-x-4 w-full">
            <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
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
        </n-form-item>
      </div>
    </n-form>

    <GButton color="green" v-on:click="onClickCreate">
      <i class="fa-regular fa-square-plus"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.create') }}</span>
    </GButton>
  </div>
</template>

<script setup>
import {ref, defineModel, defineProps} from "vue"
import {trans} from "../../../i18n/i18n";
import GButton from "../../../components/GButton.vue";
import {
  NCard,
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
  isArrayNotEmptyValidator,
} from "../../../parts/validators"
import Loading from "../../../components/Loading.vue";

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
})

const formRef = ref({})
const form = defineModel({
  name: '',
  password: '',
  passwordConfirmation: '',
})

const rules = {
  login: {
    required: true,
    validator: requiredValidator(trans('labels.login')),
  },
  email: {
    required: true,
    validator: requiredValidator(trans('labels.email')),
  },
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
  password: {
    required: true,
    validator: allOfValidator(
        requiredValidator(trans('labels.password')),
        stringMinLengthValidator(trans('labels.password'), 8),
        sameWithValidator(
            trans('labels.password'),
            trans('labels.password_confirmation'),
            () => form.value.passwordConfirmation,
        )
    ),
  },
  roles: {
    required: true,
    validator: isArrayNotEmptyValidator(trans('labels.roles'))
  }
}

const roles = [
  {
    label: "Administrator",
    value: "admin",
  },
  {
    label: "User",
    value: "user",
  }
]

const emits = defineEmits(['create'])

const onClickCreate = () => {
  formRef.value.validate().then(() => {
    emits("create")
  })
}

</script>