<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" v-on:click="onClickCreate()">
    <i class="fa fa-plus-square mr-0.5"></i>
    <span>{{ trans('users.create')}}</span>
  </GButton>

  <n-data-table
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="usersData"
      :loading="loading"
      :pagination="pagination"
  >
    <template #loading>
      <Loading />
    </template>
  </n-data-table>

  <n-modal
      v-model:show="showUserModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('users.user')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <n-table :bordered="false" :single-line="true">
      <tbody>
        <tr>
          <td><strong>{{ trans('users.login') }}:</strong></td>
          <td>{{ userStore.user.login }}</td>
        </tr>
        <tr>
          <td><strong>Email:</strong></td>
          <td>{{ userStore.user.email }}</td>
        </tr>
        <tr>
          <td><strong>{{ trans('users.name') }}:</strong></td>
          <td>{{ userStore.user.name }}</td>
        </tr>
        <tr>
          <td><strong>{{ trans('users.roles') }}:</strong></td>
          <td>{{ userStore.user.roles.join(', ')  }}</td>
        </tr>
      </tbody>
    </n-table>
  </n-modal>

  <n-modal
      v-model:show="createUserModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('users.create')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <CreateUserForm v-model="createUserModel" v-on:create="onCreate" />
  </n-modal>
</template>

<script setup>
import GBreadcrumbs from "@/components/GBreadcrumbs.vue";
import {trans} from "@/i18n/i18n";
import {computed, h, ref, onMounted} from "vue"
import {useUserListStore} from "@/store/userList";
import {useUserStore} from "@/store/user";
import {useAuthStore} from "@/store/auth";
import {storeToRefs} from "pinia"
import {errorNotification, notification} from "@/parts/dialogs";
import {
  NModal,
  NTable,
  NDataTable,
} from "naive-ui"
import Loading from "@/components/Loading.vue";
import GButton from "@/components/GButton.vue";
import CreateUserForm from "./forms/CreateUserForm.vue";

const userListStore = useUserListStore()
const userStore = useUserStore()
const authStore = useAuthStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.users.index'}, 'text':trans('users.users')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('users.name'),
      key: "name"
    },
    {
      title: "Email",
      key: "email"
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'green',
            size: 'small',
            class: 'mr-0.5',
            onClick: () => {onClickShow(row.id)},
          }, [
            h("i", {class: 'fa-solid fa-eye'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.view')),
          ]),
          h(GButton, {
            color: 'blue',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.users.edit', params: {id: row.id}},
          }, [
            h("i", {class: 'fa-solid fa-pen-to-square'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.edit')),
          ]),
          h(GButton, {
            color: 'red',
            size: 'small',
            disabled: authStore.user?.id === row.id,
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

const {users} = storeToRefs(userListStore)

const columns = ref(createColumns())
const pagination = {
  pageSize: 20,
}

const loading = computed(() => {
  return userListStore.loading || userStore.loading
})

onMounted(() => {
  fetchUsers()
})

const fetchUsers = () => {
  userListStore.fetchUsers().catch((error) => {
    errorNotification(error)
  })
}

const usersData = computed(() => {
  return users.value.map((user) => {
    return {
      id: user.id,
      name: user.name,
      email: user.email,
    }
  })
})

const onClickDelete = (id) => {
  window.$dialog.success({
    title: trans('users.delete_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteUserById(id)
    },
    onNegativeClick: () => {}
  })
}

const deleteUserById = (id) => {
  if (authStore.user?.id === id) {
    errorNotification(trans('users.delete_self_error_msg'))
    return
  }

  userListStore.deleteUserById(id).then(() => {
    notification({
      content: trans('users.delete_success_msg'),
      type: "success",
    }, () => {
      fetchUsers()
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

const showUserModalEnabled = ref(false)

const onClickShow = (id) => {
  userStore.setUserId(id)
  userStore.fetchUser().then(() => {
    showUserModalEnabled.value = true
  }).catch((error) => {
    errorNotification(error)
  })
}

const createUserModalEnabled = ref(false)
const createUserModel = ref({
  name: '',
  email: '',
})
const onClickCreate = () => {
  createUserModel.value = {}
  createUserModalEnabled.value = true
}

const onCreate = () => {
  const fields = {
    login: createUserModel.value.login,
    email: createUserModel.value.email,
    password: createUserModel.value.password,
    name: createUserModel.value.name,
    roles: createUserModel.value.roles,
  }
  userListStore.createUser(fields).then(() => {
    notification({
      content: trans('users.create_success_msg'),
      type: "success",
    }, () => {
      createUserModalEnabled.value = false
      fetchUsers()
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

</script>