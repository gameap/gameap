<template>
  <GModal
      v-model:show="visible"
      :title="trans('plugins.permissions')"
      style="width: 700px; max-width: 90vw;"
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

        <div
          v-if="missingPermissions.length > 0"
          class="mb-3 p-2 rounded-lg bg-warning-soft text-warning-soft-text text-sm break-words"
          data-testid="plugin-permissions-missing"
        >
          {{ trans('plugins.permissions_missing_warning') }}
          <span class="font-medium">{{ missingPermissions.map(permissionLabel).join(', ') }}</span>
        </div>

        <n-checkbox-group v-model:value="selectedPermissions">
          <ul class="grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-1">
            <li v-for="permission in PLUGIN_PERMISSIONS" :key="permission" class="flex items-start gap-2">
              <n-checkbox :value="permission" :data-testid="`plugin-permission-${permission}`">
                <span class="text-sm">{{ permissionLabel(permission) }}</span>
              </n-checkbox>
              <span class="flex gap-1 pt-0.5">
                <span
                  v-if="requiredPermissions.includes(permission)"
                  class="px-1.5 py-0.5 text-[10px] font-medium rounded-full bg-info-soft text-info-soft-text whitespace-nowrap"
                  :title="trans('plugins.required_permissions')"
                >
                  {{ trans('plugins.permissions_declared') }}
                </span>
                <span
                  v-if="usedPermissions.includes(permission)"
                  class="px-1.5 py-0.5 text-[10px] font-medium rounded-full whitespace-nowrap"
                  :class="missingPermissions.includes(permission)
                    ? 'bg-warning-soft text-warning-soft-text'
                    : 'bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300'"
                >
                  {{ trans('plugins.permissions_used') }}
                </span>
              </span>
            </li>
          </ul>
        </n-checkbox-group>
      </div>
    </n-spin>

    <template #footer>
      <div class="flex justify-end gap-2">
        <GButton color="white" @click="close">{{ trans('main.close') }}</GButton>
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
import { NCheckbox, NCheckboxGroup, NSpin } from 'naive-ui'
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
const { loadedPlugins } = storeToRefs(pluginStore)

const visible = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value)
})

// The row from the table carries only missing_permissions, so the grants are
// read from the loaded record the store keeps up to date after a save.
const loadedInfo = computed(() => loadedPlugins.value.find(p => p.id === props.plugin?.id) || null)

const requiredPermissions = computed(() => loadedInfo.value?.required_permissions ?? [])
const allowedPermissions = computed(() => loadedInfo.value?.allowed_permissions ?? [])
const usedPermissions = computed(() => loadedInfo.value?.used_permissions ?? [])
const missingPermissions = computed(() => loadedInfo.value?.missing_permissions ?? [])

const selectedPermissions = ref([])
const saving = ref(false)

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

function save() {
  if (!props.plugin || saving.value) return

  saving.value = true
  pluginStore.updatePluginPermissions(props.plugin.id, [...selectedPermissions.value])
      .then(() => {
        notification({
          content: trans('plugins.permissions_saved'),
          type: 'success'
        })
        close()
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
