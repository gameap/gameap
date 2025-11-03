<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <n-tabs type="line" class="flex justify-between" animated>
        <n-tab-pane name="main">
          <template #tab>
            {{ trans('games.main') }}
          </template>

          <div class="flex flex-wrap justify-center mt-2">
            <div class="md:w-1/2">
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
                <n-form-item :label="trans('labels.name')" path="name">
                  <n-input
                      v-model:value="form.name"
                      type="text"
                  />
                </n-form-item>
              </n-card>
            </div>
          </div>

          <div class="flex flex-wrap mt-2">
            <div class="md:w-1/2 pr-8">
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
              </n-card>
            </div>

            <div class="md:w-1/2">
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
              </n-card>
            </div>
          </div>

          <div class="flex flex-wrap mt-2">
            <div class="md:w-full">
              <n-card
                  :title="trans('games.run_commands')"
                  size="small"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
              >
                <n-form-item :label="trans('labels.start_cmd_linux')" path="startCmdLinux">
                  <n-input
                      v-model:value="form.startCmdLinux"
                      type="textarea"
                      :autosize="{ minRows: 4}"
                  />
                </n-form-item>

                <n-form-item :label="trans('labels.start_cmd_windows')" path="startCmdWindows">
                  <n-input
                      v-model:value="form.startCmdWindows"
                      type="textarea"
                      :autosize="{ minRows: 4}"
                  />
                </n-form-item>
              </n-card>
            </div>
          </div>
        </n-tab-pane>

        <n-tab-pane name="commands">
          <template #tab>
            {{ trans('games.servers_commands') }}
          </template>

          <div class="flex flex-wrap mt-2">
            <div class="md:w-full">
              <n-card
                  :title="trans('games.servers_commands')"
                  size="small"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
              >
                <div class="flex grid grid-cols-2 gap-x-5 gap-y-4">
                  <n-form-item :label="trans('labels.kick_cmd')" path="kickCmd">
                    <n-input
                        v-model:value="form.kickCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_kick_cmd')"></small>
                    </template>
                  </n-form-item>

                  <n-form-item :label="trans('labels.ban_cmd')" path="banCmd">
                    <n-input
                        v-model:value="form.banCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_ban_cmd')"></small>
                    </template>
                  </n-form-item>

                  <n-form-item :label="trans('labels.chname_cmd')" path="chnameCmd">
                    <n-input
                        v-model:value="form.chnameCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_chname_cmd')"></small>
                    </template>
                  </n-form-item>

                  <n-form-item :label="trans('labels.srestart_cmd')" path="srestartCmd">
                    <n-input
                        v-model:value="form.srestartCmd"
                        type="text"
                    />
                  </n-form-item>

                  <n-form-item :label="trans('labels.chmap_cmd')" path="chmapCmd">
                    <n-input
                        v-model:value="form.chmapCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_chmap_cmd')"></small>
                    </template>
                  </n-form-item>

                  <n-form-item :label="trans('labels.sendmsg_cmd')" path="sendmsgCmd">
                    <n-input
                        v-model:value="form.sendmsgCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_sendmsg_cmd')"></small>
                    </template>
                  </n-form-item>

                  <n-form-item :label="trans('labels.passwd_cmd')" path="passwdCmd">
                    <n-input
                        v-model:value="form.passwdCmd"
                        type="text"
                    />
                    <template #feedback>
                      <small v-html="trans('games.d_passwd_cmd')"></small>
                    </template>
                  </n-form-item>
                </div>

              </n-card>
            </div>
          </div>
        </n-tab-pane>

        <n-tab-pane name="vars">
          <template #tab>
            {{ trans('games.vars') }}
          </template>

          <InputManyList
              name="vars"
              v-model="form.vars"
              class="mb-4"
              :labels="[
                  trans(('games.var')),
                  trans(('games.default')),
                  trans(('games.info')),
                  trans(('games.admin_var')),
                ]"
              :keys="['var', 'default', 'info', 'admin_var']"
              :input-types="['text', 'text', 'text', 'checkbox']"
          />
        </n-tab-pane>

        <n-tab-pane name="rcon">
          <template #tab>
            {{ trans('games.fast_rcon_commands') }}
          </template>
          <InputManyList
              name="vars"
              v-model="form.fastRcon"
              class="mb-4"
              :labels="[
                  trans(('games.description')),
                  trans(('games.rcon_command')),
                ]"
              :keys="['info', 'command']"
              :input-types="['text', 'text']"
          />
        </n-tab-pane>
      </n-tabs>
    </n-form>

    <GButton color="green" v-on:click="onClickUpdate">
      <i class="fa-regular fa-floppy-disk mr-0.5"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
    </GButton>

  </div>
</template>

<script setup>
import {computed, ref, onMounted, h} from "vue"
import {allOfValidator, requiredValidator, stringLengthValidator} from "../../../parts/validators";
import {trans} from "../../../i18n/i18n";
import {
  NCard,
  NForm,
  NFormItem,
  NInput,
  NTabs,
  NTabPane,
} from "naive-ui"
import GButton from "../../../components/GButton.vue";

const formRef = ref({})
const form = defineModel({})

const rules = {
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