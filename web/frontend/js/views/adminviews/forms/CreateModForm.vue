<template>
  <div>
    <n-form
        label-placement="top"
        label-width="auto"
        ref="modCreateFormRef"
        :model="modCreateForm"
        :rules="rules"
    >
      <n-form-item :label="trans('labels.game_id')" path="game">
        <n-select
            filterable
            v-model:value="modCreateForm.game"
            :options="gameOptions"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.name')" path="name">
        <n-input
            v-model:value="modCreateForm.name"
            type="text"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.remote_repository_linux')" path="remoteRepositoryLinux">
        <n-input
            v-model:value="modCreateForm.remoteRepositoryLinux"
            type="text"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.remote_repository_windows')" path="remoteRepositoryWindows">
        <n-input
            v-model:value="modCreateForm.remoteRepositoryWindows"
            type="text"
        />
      </n-form-item>
    </n-form>

    <GButton color="green" v-on:click="onClickSendCreateModForm">
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
  NSelect,
} from "naive-ui"
import {storeToRefs} from "pinia"
import {requiredValidator} from "../../../parts/validators"
import {useGameListStore} from "../../../store/gameList"

const gamesStore = useGameListStore()

const modCreateFormRef = ref({})
const modCreateForm = defineModel({
  game: '',
  name: '',
  remoteRepositoryLinux: '',
  remoteRepositoryWindows: '',
})

const {games} = storeToRefs(gamesStore)

const rules = {
  game: {
    required: true,
    validator: requiredValidator(trans('labels.game_id')),
  },
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
}

const emits = defineEmits(['create'])

const onClickSendCreateModForm = () => {
  modCreateFormRef.value.validate().then(() => {
    emits("create")
  })
}

const gameOptions = computed(() => {
  let options = []

  games.value.forEach((game) => {
    options.push({
      label: game.name,
      value: game.code,
    })
  })

  return options
})

</script>