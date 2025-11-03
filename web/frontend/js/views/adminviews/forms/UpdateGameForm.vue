<template>
  <div>
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <div class="flex flex-wrap mt-2">
        <div class="md:w-1/2 pr-8">
          <n-card
              :title="trans('games.basic_info')"
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
              <n-form-item :label="trans('labels.code')" path="code">
                <n-input
                    disabled
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
            </div>

          </n-card>
          <slot name="mods"></slot>
        </div>

        <div class="md:w-1/2">
          <n-card
              :title="trans('games.steam_info')"
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
              <n-form-item :label="trans('labels.steam_app_id_linux')" path="steamAppIdLinux">
                <n-input
                    v-model:value="form.steamAppIdLinux"
                    type="text"
                />
              </n-form-item>
              <n-form-item :label="trans('labels.steam_app_id_windows')" path="steamAppIdWindows">
                <n-input
                    v-model:value="form.steamAppIdWindows"
                    type="text"
                />
              </n-form-item>
              <n-form-item :label="trans('labels.steam_app_set_config')" path="steamAppSetConfig">
                <n-input
                    v-model:value="form.steamAppSetConfig"
                    type="text"
                />
              </n-form-item>
            </div>
          </n-card>

          <n-card
              :title="trans('games.repositories_local')"
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
              <n-form-item :label="trans('labels.local_repository_linux')" path="localRepositoryLinux">
                <n-input
                    v-model:value="form.localRepositoryLinux"
                    type="text"
                />
              </n-form-item>

              <n-form-item :label="trans('labels.local_repository_windows')" path="localRepositoryWindows">
                <n-input
                    v-model:value="form.localRepositoryWindows"
                    type="text"
                />
              </n-form-item>
            </div>
          </n-card>
          <n-card
              :title="trans('games.repositories_remote')"
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
            </div>
          </n-card>
        </div>
      </div>
    </n-form>

    <GButton color="green" v-on:click="onClickUpdate">
      <i class="fa-regular fa-floppy-disk mr-0.5"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
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
} from "naive-ui"
import {allOfValidator, requiredValidator, stringLengthValidator} from "../../../parts/validators"
import Loading from "../../../components/Loading.vue";

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
})

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

const emits = defineEmits(['update'])

const onClickUpdate = () => {
  formRef.value.validate().then(() => {
    emits("update")
  })
}

</script>