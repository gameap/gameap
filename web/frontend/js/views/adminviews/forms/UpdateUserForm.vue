<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <div class="flex flex-wrap mt-2 grid grid-cols-2 gap-8">
        <div>
          <n-card
              :title="trans('users.user')"
              size="small"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
          >
            <Loading v-if="loading"></Loading>
            <div :class="loading ? 'hidden' : ''">
              <n-form-item :label="trans('labels.login')" path="login">
                <n-input
                    disabled
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
            </div>
          </n-card>
        </div>
        <div>
          <n-card
              :title="trans('users.update_password')"
              size="small"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
          >
            <Loading v-if="loading"></Loading>
            <div :class="loading ? 'hidden' : ''">
              <n-form-item :label="trans('labels.password')" path="password">
                <div class="gap-x-4 w-full">
                  <n-input
                      v-model:value="form.password"
                      type="password"
                      show-password-on="click"
                      :input-props="{ autocomplete: 'one-time-code' }"
                  />

                  <n-input
                      class="mt-6"
                      v-model:value="form.passwordConfirmation"
                      type="password"
                      show-password-on="click"
                      :input-props="{ autocomplete: 'one-time-code' }"
                      :placeholder="trans('labels.password_confirmation')"
                  />
                </div>
              </n-form-item>
            </div>
          </n-card>
        </div>
      </div>

      <div class="flex flex-wrap mt-2">
        <div class="md:w-full">
          <n-card
              :title="trans('users.servers')"
              size="small"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
          >
            <Loading v-if="loading"></Loading>
            <div :class="loading ? 'hidden' : ''">
              <UserServerPrivileges
                  :user-id="userStore.userId"
                  v-model="form.servers"
              />
            </div>
          </n-card>
        </div>
      </div>
    </n-form>

    <GButton color="green" v-on:click="onClickUpdate" class="mt-4">
      <i class="fa-regular fa-floppy-disk mr-0.5"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
    </GButton>
  </div>
</template>

<script setup>
import {ref, defineModel, defineProps, defineEmits} from "vue"
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
  isArrayNotEmptyValidator,
  requiredValidator,
  sameWithValidator,
  stringMinLengthValidator,
  ifNotEmptyValidator,
} from "../../../parts/validators"
import Loading from "../../../components/Loading.vue";
import {useUserStore} from "../../../store/user"

const userStore = useUserStore()

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
})

const formRef = ref({})
const form = defineModel({})

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

const emits = defineEmits(['update'])

const onClickUpdate = () => {
  formRef.value.validate().then(() => {
    emits("update")
  })
}

</script>