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
        <n-form-item :label="trans('client_certificates.certificate')" path="certificate">
          <n-upload
              :max="1"
              :multiple="false"
              :default-upload="false"
              @change="handleChangeCertificate"
          >
            <n-button>{{ trans('main.upload_file') }}</n-button>
          </n-upload>
        </n-form-item>

        <n-form-item :label="trans('client_certificates.private_key')" path="privateKey">
          <n-upload
              :max="1"
              :multiple="false"
              :default-upload="false"
              @change="handleChangePrivateKey"
          >
            <n-button>{{ trans('main.upload_file') }}</n-button>
          </n-upload>
        </n-form-item>

        <n-form-item :label="trans('client_certificates.private_key_pass')" path="privateKeyPass">
          <n-input
              v-model:value="form.privateKeyPass"
              type="password"
              show-password-on="click"
              :input-props="{ autocomplete: 'one-time-code' }"
          />
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
  NForm,
  NFormItem,
  NInput,
  NButton,
  NUpload,
} from "naive-ui"
import Loading from "../../../components/Loading.vue";

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
})

const formRef = ref({})
const form = defineModel({
  privateKeyPass: '',
})
const certificateFile = defineModel('certificateFile')
const privateKeyFile = defineModel('privateKeyFile')

const rules = {}

const handleChangeCertificate = (data) => {
  if (data.fileList.length > 0) {
    certificateFile.value = data.fileList[0].file
  } else {
    certificateFile.value = null
  }
}

const handleChangePrivateKey = (data) => {
  if (data.fileList.length > 0) {
    privateKeyFile.value = data.fileList[0].file
  } else {
    privateKeyFile.value = null
  }
}

const emits = defineEmits(['create'])

const onClickCreate = () => {
  formRef.value.validate().then(() => {
    emits("create")
  })
}
</script>