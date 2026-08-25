<template>
  <GModal
      v-model:show="visible"
      :title="trans('plugins.permissions')"
      style="width: 780px; max-width: 90vw;"
  >
    <n-spin :show="saving">
      <div v-if="loadedInfo" data-testid="plugin-permissions">
        <div class="flex items-center gap-3 mb-3">
          <PluginIcon :plugin="plugin" img-class="w-10 h-10 rounded" fallback-class="text-3xl text-stone-400" />
          <div class="min-w-0">
            <div class="font-medium break-words">{{ plugin.name }}</div>
            <p class="text-xs text-stone-500">{{ trans('plugins.permissions_hint') }}</p>
          </div>
        </div>

        <n-alert
          v-if="!permissionsEnforced"
          type="warning"
          :show-icon="true"
          class="mb-3"
          data-testid="plugin-permissions-enforcement-disabled"
        >
          {{ trans('plugins.permissions_enforcement_disabled_warning') }}
        </n-alert>

        <n-alert
          v-if="permissionsUnknown"
          type="default"
          :show-icon="true"
          class="mb-3"
          data-testid="plugin-permissions-unknown"
        >
          {{ trans('plugins.permissions_unknown') }}
        </n-alert>

        <n-alert
          v-else-if="missingPermissions?.length > 0"
          type="warning"
          :show-icon="true"
          class="mb-3"
          data-testid="plugin-permissions-missing"
        >
          {{ trans('plugins.permissions_missing_warning') }}
          <span class="font-medium">{{ missingPermissions.map(permissionLabel).join(', ') }}</span>
        </n-alert>

        <n-checkbox-group v-model:value="selectedPermissions">
          <ul class="grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-1">
            <li v-for="permission in PLUGIN_PERMISSIONS" :key="permission">
              <n-checkbox
                :value="permission"
                :disabled="permissionDisabled(permission)"
                :title="permissionDisabled(permission) ? trans('plugins.permission_not_needed') : undefined"
                :data-testid="`plugin-permission-${permission}`"
              >
                <span class="text-sm">{{ permissionLabel(permission) }}</span>
              </n-checkbox>
            </li>
          </ul>
        </n-checkbox-group>
      </div>
    </n-spin>

    <template #footer>
      <div class="flex justify-end gap-2">
        <GButton color="white" :disabled="saving" @click="close">{{ trans('main.close') }}</GButton>
        <GButton
            color="blue"
            :disabled="!permissionsChanged || saving"
            data-testid="plugin-permissions-save"
            @click="save"
        >
          <GIcon name="check" class="mr-1" />
          {{ trans('plugins.permissions_save') }}
        </GButton>
      </div>
    </template>
  </GModal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { trans } from '@/i18n/i18n'
import { GIcon, GModal } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import PluginIcon from '@/components/plugins/PluginIcon.vue'
import { NAlert, NCheckbox, NCheckboxGroup, NSpin } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { usePluginStoreStore } from '@/store/pluginStore'
import { errorNotification, notification } from '@/parts/dialogs'
import { PLUGIN_PERMISSIONS } from '@/parts/pluginPermissions'

const props = defineProps({
  show: {
    type: Boolean,
    default: false
  },
  plugin: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['update:show'])

const pluginStore = usePluginStoreStore()
const { loadedPlugins, permissionsEnforced } = storeToRefs(pluginStore)

const visible = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value)
})

// The row from the table carries only missing_permissions, so the grants are
// read from the loaded record the store keeps up to date after a save.
const loadedInfo = computed(() => loadedPlugins.value.find(p => p.id === props.plugin?.id) || null)

const requiredPermissions = computed(() => loadedInfo.value?.required_permissions ?? [])
const allowedPermissions = computed(() => loadedInfo.value?.allowed_permissions ?? [])

// Null (not []) when the instance that answered has not loaded the plugin: it
// cannot tell what the module exercises. Kept null so "unknown" is not
// mistaken for "uses nothing gated" — the former shows a notice and locks no
// checkbox, the latter locks everything the plugin does not need.
const usedPermissions = computed(() => loadedInfo.value?.used_permissions ?? null)
const missingPermissions = computed(() => loadedInfo.value?.missing_permissions ?? null)
const permissionsUnknown = computed(() => usedPermissions.value === null)

const selectedPermissions = ref([])
const saving = ref(false)

// A save can outlive the modal: it can be dismissed mid-flight (mask, esc,
// the header cross) and reopened, possibly for another plugin. Every opening
// starts a new generation, and only a completion from the current one is
// allowed to close the form.
let modalGeneration = 0

// The checkboxes follow the record; a saved update flows back through
// loadedInfo, an unsaved edit is dropped when the modal is opened again.
// While the form is open it is left alone: a fetchLoadedPlugins() landing
// mid-edit (a row reload started before the modal opened) replaces the
// record and would otherwise reset the checkboxes under the operator.
watch(allowedPermissions, (allowed) => {
  if (props.show) return

  selectedPermissions.value = [...allowed]
}, { immediate: true })

watch(() => props.show, (shown) => {
  if (shown) {
    modalGeneration++
    selectedPermissions.value = [...allowedPermissions.value]
  }
})

const permissionsChanged = computed(() => {
  const current = [...selectedPermissions.value].sort()
  const saved = [...allowedPermissions.value].sort()

  return current.length !== saved.length || current.some((permission, i) => permission !== saved[i])
})

function permissionLabel(permission) {
  return trans('plugins.permission_' + permission)
}

// A permission the plugin neither declares nor uses cannot be granted. A
// grant that is already saved stays editable so it can be revoked, and when
// usage is unknown nothing is locked: "not used" cannot be established.
function permissionDisabled(permission) {
  if (permissionsUnknown.value) return false

  return !requiredPermissions.value.includes(permission)
      && !usedPermissions.value.includes(permission)
      && !allowedPermissions.value.includes(permission)
}

function save() {
  if (!props.plugin || saving.value) return

  const generation = modalGeneration

  saving.value = true
  pluginStore.updatePluginPermissions(props.plugin.id, [...selectedPermissions.value])
      .then(() => {
        notification({
          content: trans('plugins.permissions_saved'),
          type: 'success'
        })

        if (generation === modalGeneration) {
          close()
        }
      })
      .catch(errorNotification)
      .finally(() => {
        saving.value = false
      })
}

function close() {
  visible.value = false
}
</script>
