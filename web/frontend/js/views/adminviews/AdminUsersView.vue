<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5" v-on:click="onClickCreate()">
    <GIcon name="add-square" class="mr-0.5" />
    <span>{{ trans('users.create')}}</span>
  </GButton>

  <GDataTable
      ref="tableRef"
      :columns="columns"
      :data="usersData"
      :loading="loading"
      :pagination="pagination"
      :scroll-x="560"
  >
    <template #loading>
      <Loading />
    </template>
  </GDataTable>

  <GModal
      v-model:show="showUserModalEnabled"
      :title="trans('users.user')"
      style="width: 600px"
  >
    <GTable>
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
    </GTable>

    <PluginSlot
        v-if="pluginsStore.isInitialized"
        name="admin-user-info"
        :context="{ userId: userStore.userId, user: userStore.user }"
    />
  </GModal>

  <GModal
      v-model:show="createUserModalEnabled"
      :title="trans('users.create')"
      style="width: 600px"
  >
    <CreateUserForm v-model="createUserModel" v-on:create="onCreate" />
  </GModal>
</template>

<script setup>
import { GBreadcrumbs, Loading, GIcon, GDataTable, GModal, GTable } from "@gameap/ui";
import {trans} from "@/i18n/i18n";
import {computed, h, ref, onMounted} from "vue"
import {useUserListStore} from "@/store/userList";
import {useUserStore} from "@/store/user";
import {useAuthStore} from "@/store/auth";
import {usePluginsStore} from "@/store/plugins";
import {storeToRefs} from "pinia"
import {errorNotification, notification} from "@/parts/dialogs";
import GButton from "@/components/GButton.vue";
import CreateUserForm from "./forms/CreateUserForm.vue";
import PluginSlot from "@/plugins/components/PluginSlot.vue";

const userListStore = useUserListStore()
const userStore = useUserStore()
const authStore = useAuthStore()
const pluginsStore = usePluginsStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
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
          }, { default: () => [
            h(GIcon, {name: 'view'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.view')),
          ]}),
          h(GButton, {
            color: 'blue',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.users.edit', params: {id: row.id}},
          }, { default: () => [
            h(GIcon, {name: 'edit'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.edit')),
          ]}),
          h(GButton, {
            color: 'red',
            size: 'small',
            disabled: authStore.user?.id === row.id,
            text: trans('main.delete'),
            onClick: () => {onClickDelete(row.id)},
          }, { default: () => [
            h(GIcon, {name: 'delete'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
          ]}),
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