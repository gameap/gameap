<template>
    <div>
        <div class="mb-4">
            <n-table :bordered="false" :single-line="true">
                <thead>
                  <tr>
                      <th>{{ trans('servers.name') }}</th>
                      <th>{{ trans('servers.game') }}</th>
                      <th>{{ trans('servers.ip_port') }}</th>
                      <th>{{ trans('main.actions') }}</th>
                  </tr>
                </thead>

                <tbody>
                  <tr v-for="(server, itemIndex) in items">
                      <td><a :href="'/admin/servers/' + server.id + '/edit'">{{ server.name }}</a></td>
                      <td>
                        <GameIcon :game="server.game.code" />
                        {{ server.game.name }}
                      </td>
                      <td>{{ server.server_ip }}:{{ server.server_port }}</td>
                      <td class="flex flex-wrap gap-1">
                        <GButton color="blue" size="small" v-on:click="onClickEditPrivileges(server)">
                          <i class="fas fa-lock"></i>
                        </GButton>
                        <GButton color="red" size="small" v-on:click="removeItem(itemIndex)">
                            <i class="fa fa-times"></i>
                        </GButton>
                      </td>
                  </tr>
                </tbody>
            </n-table>
        </div>

        <div class="mt-6 mb-3">
          <div class="flex justify-center mt-2">
            <GButton color="green" size="small" v-on:click="onClickAddServer">
              <span class="fa-solid fa-plus"></span>&nbsp;{{ trans('main.add') }}
            </GButton>
          </div>
        </div>
    </div>

  <n-modal
      v-model:show="addServerModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('servers.add_server')"
      :bordered="false"
      style="width: 800px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <div class="md:w-full">
      <n-select
          v-model:value="selectedServer"
          filterable
          :placeholder="trans('users.servers_privileges_placeholder')"
          :options="serversListOptions"
          :render-label="renderLabel"
          :loading="loading"
          clearable
          remote
          @search="onSearch"
      >
        <template #action>
          <div>
            <div class="m-2">
              <span>...</span>
            </div>
          </div>
        </template>
      </n-select>
    </div>

    <div class="flex justify-center mt-2">
      <GButton color="green" v-on:click="addItem">
        <span class="fa-solid fa-plus"></span>&nbsp;{{ trans('main.add') }}
      </GButton>
    </div>
  </n-modal>

  <n-modal
      v-model:show="editPrivilegestModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('users.server_permission_edit')"
      :bordered="false"
      style="width: 800px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <n-table :bordered="false" :single-line="true" size="small" class="mb-8">
      <tbody>
      <tr>
        <td><strong>{{ trans('servers.name') }}:</strong></td>
        <td>{{ editPrivilegesServer.name }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('servers.ip_port') }}:</strong></td>
        <td>{{ editPrivilegesServer.server_ip }}:{{ editPrivilegesServer.server_port }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('servers.game') }}:</strong></td>
        <td>
          <GameIcon :game="editPrivilegesServer.game.code" class="mr-2" />
          {{ editPrivilegesServer.game.name }}
        </td>
      </tr>
      </tbody>
    </n-table>

    <div class="grid grid-cols-2 gap-x-20 mb-4">
      <div v-for="item in serverPermissions" class="grid grid-cols-4 gap-x-4 mb-6">
        <div class="col-span-3">{{ trans(item.name) }}</div>
        <n-switch
            v-model:value="item.value"
        />
      </div>
    </div>

    <GButton color="green" v-on:click="onClickSavePermissions">
      <i class="fa-solid fa-floppy-disk"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
    </GButton>

  </n-modal>
</template>

<script setup>
import { defineProps, defineModel, ref, onMounted, computed, h } from 'vue'
import axios from '../../config/axios'
import {storeToRefs} from "pinia";
import {
  NTable,
  NSelect,
  NModal,
  NSwitch,
} from "naive-ui"
import GButton from "../GButton.vue";
import GameIcon from "../GameIcon.vue";
import {trans} from "../../i18n/i18n";
import {useUserStore} from "../../store/user";
import {errorNotification, notification} from "../../parts/dialogs";

const userStore = useUserStore()

const {getServerPermissions} = storeToRefs(userStore)

const props = defineProps({
    userId: Number,
})

onMounted(() => {
  onSearch('')
})

const items = defineModel([])

const selectedServer = ref(null)
const serverList = ref([])
const serversListOptions = ref([])
const loading = ref(false)

function existsItem(id) {
  for (let item of items.value) {
    if (item.id === id) {
      return true;
    }
  }
}

function findItem(id) {
  for (let item of serverList.value) {
    if (item.id === id) {
      return item;
    }
  }
}

function removeListsElem(elemId) {
  serverList.value = serverList.value.filter(function(elem) {
    return elem.id !== elemId;
  });

  serversListOptions.value = serversListOptions.value.filter(function(elem) {
    return elem.value !== elemId;
  });
}

function removeItem(index) {
  items.value.splice(index, 1);
}

function addItem() {
  if (!selectedServer.value) {
    notification(trans('servers.select_server'))

    return;
  }

  addServerModalEnabled.value = false;

  if (existsItem(selectedServer.value)) {
    notification(trans('servers.server_already_added'))

    return;
  }

  const newItem = findItem(selectedServer.value);

  if (newItem) {
    items.value.push(newItem);

    removeListsElem(newItem.id);
    selectedServer.value = null;
  }
}

let timer
function debounce(f) {
  clearTimeout(timer)
  timer = setTimeout(() => {
    f()
  }, 500)
}

function onSearch(search) {
  loading.value = true;

  debounce(() => {
    axios.get(`/api/servers/search?q=${encodeURI(search)}`)
        .then(function (response) {
          serverList.value = [];
          serversListOptions.value = [];

          for (let item of response.data) {
            if (existsItem(item.id)) {
              continue;
            }

            serverList.value.push({
              id: item.id,
              name: item.name,
              game: item.game,
              server_ip: item.server_ip,
              server_port: item.server_port
            });

            serversListOptions.value.push({
              name: item.name,
              label: item.name + ' (' + item.game.name + ')' + ' - ' + item.server_ip + ':' + item.server_port,
              address: `${item.server_ip}:${item.server_port}`,
              game: item.game,
              value: item.id,
            });
          }

          loading.value = false;
        })
        .catch(function (error) {
          loading.value = false;
          errorNotification(error)
        });
  })
}

const renderLabel = (option) => {
  return h(
      "div",
      { class: "flex" },
      [
        h("div", {class: "w-64"}, option.name),
        h("div", {class: "w-56"}, option.game.name),
        h("div", {class: "w-40"}, option.address),
      ]
  )
}

const addServerModalEnabled = ref(false);
const onClickAddServer = () => {
  addServerModalEnabled.value = true;
}

const editPrivilegestModalEnabled = ref(false);
const onClickEditPrivileges = (server) => {
  userStore.fetchPermissionsForServer(server.id).then(() => {
    editPrivilegesServer.value = server;
    editPrivilegestModalEnabled.value = true;
  }).catch((error) => {
    errorNotification(error)
  });
}

const editPrivilegesServer = ref({})
const serverPermissions = computed({
  get: () => {
    if (!editPrivilegesServer.value.id) {
      return []
    }
    return getServerPermissions.value(editPrivilegesServer.value.id)
  },
  set: (value) => {
    userStore.setServerPermissions(editPrivilegesServer.value.id, value)
  },
})

const onClickSavePermissions = () => {
  userStore.savePermissionsForServer(editPrivilegesServer.value.id).then(() => {
    notification({
      content: trans('users.privileges_saved_success_msg'),
      type: "success",
    }, () => {
      editPrivilegestModalEnabled.value = false;
    })
  }).catch((error) => {
    errorNotification(error)
  });
}

</script>
