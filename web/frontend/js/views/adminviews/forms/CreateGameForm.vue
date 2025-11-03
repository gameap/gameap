<template>
  <div>
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <n-form-item :label="trans('labels.code')" path="code">
        <n-input
            minlength="2"
            maxlength="16"
            show-count
            v-model:value="form.code"
            type="text"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.name')" path="name">
        <n-input
            v-model:value="form.name"
            type="text"
        />
      </n-form-item>

      <n-space>
        <n-form-item :label="trans('labels.engine')" path="engine">
          <n-input
              v-model:value="form.engine"
              type="text"
          />
        </n-form-item>
        <n-form-item :label="trans('labels.engine_version')" path="engineVersion">
          <n-input
              v-model:value="form.engineVersion"
              type="text"
          />
        </n-form-item>
      </n-space>

      <n-form-item :label="trans('labels.remote_repository_linux')" path="remoteRepositoryLinux">
        <n-input
            v-model:value="form.remoteRepositoryLinux"
            type="text"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.remote_repository_windows')" path="remoteRepositoryWindows">
        <n-input
            v-model:value="form.remoteRepositoryWindows"
            type="text"
        />
      </n-form-item>
    </n-form>

    <GButton color="green" v-on:click="onClickCreate">
      <i class="fa-regular fa-square-plus"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.create') }}</span>
    </GButton>
  </div>
</template>

<script setup>
import {computed, ref, defineModel} from "vue"
import {trans} from "../../../i18n/i18n";
import GButton from "../../../components/GButton.vue";
import {
  NForm,
  NFormItem,
  NInput,
  NSpace,
} from "naive-ui"
import {allOfValidator, requiredValidator, stringLengthValidator} from "../../../parts/validators"

const formRef = ref({})
const form = defineModel({
  code: '',
  name: '',
  engine: '',
  engineVersion: '',
  remoteRepositoryLinux: '',
  remoteRepositoryWindows: '',
})

const rules = {
  code: {
    required: true,
    validator: allOfValidator(
        requiredValidator(trans('labels.code')),
        stringLengthValidator(trans('labels.code'), 2, 16)
    ),
  },
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
}

const emits = defineEmits(['create'])

const onClickCreate = () => {
  formRef.value.validate().then(() => {
    emits("create")
  })
}


</script>