<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <UpdateUserForm
      :loading="loading"
      v-model="userUpdateModel"
      v-on:update="onClickUpdate"
  />
</template>

<script setup>
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import UpdateUserForm from "./forms/UpdateUserForm.vue"
import {computed, ref, onMounted} from "vue"
import {trans} from "../../i18n/i18n"
import {useUserStore} from "../../store/user"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import {errorNotification, notification} from "../../parts/dialogs"

const route = useRoute()
const router = useRouter()

const userStore = useUserStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.users.index'}, 'text':trans('users.users')},
    {'text': trans('users.title_edit')},
  ]
})

const {loading, user} = storeToRefs(userStore)

onMounted(() => {
  userStore.setUserId(route.params.id)
  userStore.fetchUser().then(() => {
    userUpdateModel.value.login = userStore.user.login
    userUpdateModel.value.email = userStore.user.email
    userUpdateModel.value.name = userStore.user.name
    userUpdateModel.value.roles = userStore.user.roles
  }).catch((error) => {
    errorNotification(error)
  })

  userStore.fetchServers().then(() => {
    userUpdateModel.value.servers = userStore.servers
  }).catch((error) => {
    errorNotification(error)
  })
})

const userUpdateModel = ref({
  login: '',
  email: '',
  name: '',
  roles: [],
  password: '',
  servers: [],
})

const onClickUpdate = () => {
  const fields = {
    login: userUpdateModel.value.login,
    email: userUpdateModel.value.email,
    name: userUpdateModel.value.name,
    roles: userUpdateModel.value.roles,
    password: userUpdateModel.value.password,
    servers: userUpdateModel.value.servers.map(object => object.id)
  }
  userStore.saveUser(fields).then(() => {
    notification({
      content: trans('users.update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.users.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

</script>