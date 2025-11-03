<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <div class="mb-5">
    <GButton class="mr-1" color="green" v-on:click="onClickGenerate()">
      <i class="fa-solid fa-plus-square"></i>&nbsp;{{ trans('tokens.generate_token') }}
    </GButton>
  </div>

  <n-data-table
      remote
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="listData"
      :loading="loading"
      :pagination="pagination"
  >
    <template #loading>
      <Loading />
    </template>
  </n-data-table>

  <n-modal
      v-model:show="generateTokenModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('tokens.generate_token')"
      :bordered="false"
      style="width: 800px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <GenerateTokenForm
        :abilities="abilities"
        v-model="generateTokenModel"
        v-on:generate="onGenerateToken"
    />
  </n-modal>
</template>

<script setup>
import {computed, onMounted, ref, h} from "vue"
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import {trans} from "@/i18n/i18n"
import Loading from "@/components/Loading.vue";
import GButton from "@/components/GButton.vue";
import {useTokensStore} from "@/store/tokens";
import {storeToRefs} from "pinia"
import _ from "lodash";
import {
  NButton,
  NDataTable,
  NModal,
  NInput,
  NInputGroup,
} from "naive-ui"
import {errorNotification, notification} from "@/parts/dialogs";
import GenerateTokenForm from "./forms/GenerateTokenForm.vue";

const tokensStore = useTokensStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'tokens'}, 'text':trans('tokens.tokens')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('tokens.name'),
      key: 'name',
    },
    {
      title: trans('tokens.abilities'),
      key: 'abilities',
      width: "30%",
    },
    {
      title: trans('tokens.last_used'),
      key: 'lastUsedAt',
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'red',
            size: 'small',
            text: trans('main.delete'),
            onClick: () => {onClickDelete(row.id)},
          }, [
            h("i", {class: 'fa-solid fa-trash'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
          ]),
        ]
      },
    }
  ]
}

const columns = ref(createColumns())
const pagination = {
  pageSize: 30,
}

const {loading, tokens, abilities} = storeToRefs(tokensStore)

onMounted(() => {
  fetchTokens()
})

const fetchTokens = () => {
  tokensStore.fetchTokens().catch((error) => {
    errorNotification(error)
  })
}

const listData = computed(() => {
  if (!Array.isArray(tokens.value)) {
    return []
  }

  return tokens.value.map((token) => {
    return {
      id: token.id,
      name: token.name,
      abilities: _.join(token.abilities, ', '),
      createdAt: (new Date(token.created_at)).toLocaleString(),
      lastUsedAt: token.last_used_at
          ? (new Date(token.last_used_at)).toLocaleString()
          : trans('main.never'),
    }
  })
})

const onClickGenerate = () => {
  tokensStore.fetchAbilities().then(() => {
    generateTokenModel.value = {
      name: '',
      abilities: [],
    }

    generateTokenModalEnabled.value = true
  }).catch((error) => {
    errorNotification(error)
  })
}

const generateTokenModalEnabled = ref(false)
const generateTokenModel = ref({
  name: '',
  abilities: [],
})

const onGenerateToken = () => {
  tokensStore.createToken(
      generateTokenModel.value.name,
      generateTokenModel.value.abilities,
  ).then((result) => {
    const copied = ref(false)

    notification({
      content: () => {
        return [
          h('div', {class: 'my-2'},
              trans('tokens.token_created_notification'),
          ),
          h(NInputGroup, {
            class: "mt-2 mb-4",
          }, [
            h(NInput, {
              value: result.token,
              readonly: true,
              size: 'small',
              style: 'width: 100%',
            }),
            h(NButton, {
              type: "primary",
              onClick: () => {
                navigator.clipboard.writeText(result.token).then(() => {
                  copied.value = true
                },() => {
                  console.error('Failed to copy');
                });
              },
            }, [
                copied.value
                    ? h("i", {class: "fa-solid fa-check" })
                    : h("i", {class: "fa-solid fa-copy" }),
            ])
          ]),
        ]
      },
      style: {
        width: '600px',
      },
      maskClosable: false,
      type: "success",
    }, () => {

    })

    fetchTokens()
    generateTokenModalEnabled.value = false
    fetchTokens()
  }).catch((error) => {
    errorNotification(error)
  })
}

const onClickDelete = (id) => {
  const deleteFunc = () => {
    tokensStore.deleteToken(id).then(() => {
      notification({
        content: trans('tokens.token_removed_msg'),
        type: "success",
      })
      fetchTokens()
    }).catch((error) => {
      errorNotification(error)
    })
  }

  window.$dialog.success({
    title: trans('tokens.delete_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteFunc()
    },
    onNegativeClick: () => {}
  })
}
</script>