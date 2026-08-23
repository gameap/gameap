<template>
  <div class="plugin-details">
    <div v-if="loading && !plugin" class="flex justify-center py-8">
      <Loading />
    </div>

    <div v-else-if="plugin">
      <div class="flex items-start gap-4 mb-4">
        <PluginIcon
            :plugin="plugin"
            img-class="w-16 h-16 rounded-lg"
            fallback-icon="puzzle-piece"
            fallback-class="text-5xl text-stone-400"
        />

        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h2 class="text-xl font-bold break-words">{{ plugin.name }}</h2>
            <GIcon v-if="plugin.requires_subscription" name="star" class="text-yellow-500" />
            <a v-if="plugin.url" :href="plugin.url" target="_blank" class="text-info hover:text-info-hover">
              <GIcon name="external-link" />
            </a>
            <span v-if="plugin.installed" class="hidden md:inline px-2 py-0.5 text-xs font-medium rounded-full bg-success-soft text-success-soft-text whitespace-nowrap">
              {{ trans('plugins.already_installed') }}
            </span>
            <span
              v-if="loadedInfo?.source_type"
              class="hidden md:inline px-2 py-0.5 text-xs font-medium rounded-full whitespace-nowrap"
              :class="loadedInfo.source_type === 'file'
                ? 'bg-info-soft text-info-soft-text'
                : 'bg-success-soft text-success-soft-text'"
            >
              {{ loadedInfo.source_type === 'file' ? trans('plugins.source_file') : trans('plugins.source_store') }}
            </span>
            <span
              v-if="loadedInfo"
              class="hidden md:inline px-2 py-0.5 text-xs font-medium rounded-full whitespace-nowrap"
              :class="loadedStatusClass"
              :title="loadedStatus === 'error' ? loadedInfo.error : undefined"
            >
              {{ trans('plugins.status_' + loadedStatus) }}
            </span>
          </div>

          <div
            v-if="loadedStatus === 'error' && loadedInfo?.error"
            class="mb-4 p-2 rounded-lg bg-danger-soft text-danger-soft-text text-sm break-words"
          >
            <span class="font-medium">{{ trans('plugins.last_error') }}:</span>
            {{ loadedInfo.error }}
            <span v-if="loadedInfo.error_at" class="text-xs opacity-75">({{ formatDateTime(loadedInfo.error_at) }})</span>
          </div>

          <div v-if="plugin.summary" class="mb-4 text-stone-600 dark:text-stone-400">
            {{ plugin.summary }}
          </div>

          <div v-if="plugin.labels?.length > 0" class="hidden md:flex flex-wrap gap-1 mt-1">
            <span
                v-for="label in plugin.labels"
                :key="label.id"
                class="px-2 py-0.5 text-xs font-medium rounded-full"
                :style="label.color ? { backgroundColor: label.color, color: '#fff' } : {}"
                :class="!label.color ? 'bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300' : ''"
            >
              {{ label.name }}
            </span>
          </div>

          <div v-if="!isFilePlugin" class="flex items-center gap-2 mt-2">
            <span class="text-orange-500 text-lg">{{ renderStars(plugin.rating_avg) }}</span>
            <a v-if="plugin.url" :href="plugin.url + '/reviews'" target="_blank" class="text-stone-500 hover:text-info hover:underline">
              ({{ plugin.rating_count || 0 }} {{ trans('plugins.reviews') }})
            </a>
            <span v-else class="text-stone-500">({{ plugin.rating_count || 0 }} {{ trans('plugins.reviews') }})</span>
          </div>
        </div>

        <div class="flex flex-col md:flex-row items-end md:items-center gap-2">
          <template v-if="!plugin.installed && requiresSubscriptionPurchase">
            <GButton
                color="orange"
                :size="isSmallScreen ? 'small' : 'medium'"
                @click="openSubscriptionUrl"
            >
              <GIcon name="external-link" class="mr-1" />
              {{ trans('plugins.get_subscription') }}
            </GButton>
          </template>
          <template v-else>
            <n-select
                v-if="!isFilePlugin && (!plugin.installed || hasUpdate)"
                v-model:value="selectedVersion"
                :options="versionOptions"
                :placeholder="trans('plugins.select_version')"
                style="width: 120px;"
                :size="isSmallScreen ? 'small' : 'large'"
            />
            <GButton
                v-if="!plugin.installed"
                color="green"
                :size="isSmallScreen ? 'small' : 'medium'"
                @click="$emit('install', selectedVersion)"
            >
              <GIcon name="download" class="mr-1" />
              {{ trans('plugins.install') }}
            </GButton>
          </template>
          <GButton
              v-if="plugin.installed && hasUpdate && !isFilePlugin"
              color="blue"
              :size="isSmallScreen ? 'small' : 'medium'"
              @click="$emit('update', selectedVersion)"
          >
            <GIcon name="sync" class="mr-1" />
            {{ trans('plugins.update') }}
          </GButton>
          <GButton
              v-if="plugin.installed"
              color="red"
              :size="isSmallScreen ? 'small' : 'medium'"
              @click="$emit('uninstall')"
          >
            <GIcon name="close" class="mr-1" />
            {{ trans('plugins.uninstall') }}
          </GButton>
        </div>
      </div>

      <div
          v-if="plugin.author
            || plugin.category
            || plugin.license
            || (plugin.download_count !== undefined && !isFilePlugin)
            || (plugin.published_at && !isFilePlugin)
            || (plugin.has_subscription && plugin.subscription_expires_at)
            || plugin.min_gameap_version
            || (plugin.url && !isFilePlugin)
          "
          class="flex flex-wrap justify-around gap-y-3 py-4 border-t border-b border-stone-200 dark:border-stone-700 mb-4"
      >
        <div v-if="plugin.author" class="flex flex-col items-center text-center px-4">
          <GIcon name="user" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ plugin.author.username }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.author') }}</span>
        </div>

        <div v-if="plugin.category" class="flex flex-col items-center text-center px-4">
          <GIcon name="folder" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ plugin.category.name }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.category') }}</span>
        </div>

        <div v-if="plugin.license" class="flex flex-col items-center text-center px-4">
          <GIcon name="scale-balanced" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ plugin.license }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.license') }}</span>
        </div>

        <div v-if="plugin.download_count !== undefined && !isFilePlugin" class="hidden md:flex flex-col items-center text-center px-4">
          <GIcon name="download" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ formatNumber(plugin.download_count) }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.downloads') }}</span>
        </div>

        <div v-if="plugin.published_at && !isFilePlugin" class="hidden md:flex flex-col items-center text-center px-4">
          <GIcon name="calendar" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ formatDate(plugin.published_at) }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.published_at') }}</span>
        </div>

        <div v-if="plugin.has_subscription && plugin.subscription_expires_at" class="flex flex-col items-center text-center px-4">
          <GIcon name="star" class="text-xl text-yellow-500 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ formatDate(plugin.subscription_expires_at) }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.subscription_expires') }}</span>
        </div>

        <div v-if="plugin.min_gameap_version" class="flex flex-col items-center text-center px-4">
          <GIcon name="gamepad" class="text-xl text-stone-500 dark:text-stone-400 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ plugin.min_gameap_version }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.min_gameap_version') }}</span>
        </div>

        <a v-if="plugin.url && !isFilePlugin" :href="plugin.url" target="_blank" class="flex flex-col items-center text-center px-4 hover:text-info transition-colors">
          <GIcon name="external-link" class="text-xl text-info mb-1" />
          <span class="text-sm font-medium text-info">{{ trans('plugins.plugin_page') }}</span>
        </a>
      </div>

      <div v-if="plugin.description" class="mb-4">
        <h3 class="font-semibold mb-2">{{ trans('plugins.description') }}</h3>
        <div class="text-sm whitespace-pre-wrap break-words">{{ plugin.description }}</div>
      </div>

      <div v-if="plugin.installed && loadedInfo" class="mb-4" data-testid="plugin-permissions">
        <h3 class="font-semibold mb-1">{{ trans('plugins.permissions') }}</h3>
        <p class="text-xs text-stone-500 mb-3">{{ trans('plugins.permissions_hint') }}</p>

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

        <div class="flex justify-end mt-3">
          <GButton
            color="blue"
            :size="isSmallScreen ? 'small' : 'medium'"
            :disabled="!permissionsChanged"
            data-testid="plugin-permissions-save"
            @click="$emit('save-permissions', [...selectedPermissions])"
          >
            <GIcon name="check" class="mr-1" />
            {{ trans('plugins.permissions_save') }}
          </GButton>
        </div>
      </div>

      <div v-if="plugin.installed" class="flex justify-center gap-6 mb-4 p-3 bg-stone-100 dark:bg-stone-800 rounded-lg">
        <div class="flex flex-col items-center text-center">
          <GIcon name="box" class="text-xl text-lime-500 mb-1" />
          <span class="text-sm font-medium whitespace-nowrap">{{ plugin.installed_version }}</span>
          <span class="text-xs text-stone-500">{{ trans('plugins.installed_version') }}</span>
        </div>
        <template v-if="!isFilePlugin">
          <div class="flex flex-col items-center text-center">
            <GIcon name="arrow-up" class="text-xl mb-1" :class="hasUpdate ? 'text-orange-500' : 'text-stone-400'" />
            <span class="text-sm font-medium whitespace-nowrap">{{ plugin.latest_version }}</span>
            <span class="text-xs text-stone-500">{{ trans('plugins.latest_version') }}</span>
          </div>
          <span v-if="hasUpdate" class="self-center px-2 py-0.5 text-xs font-medium rounded-full bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-300">
            {{ trans('plugins.update') }}
          </span>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { trans } from '@/i18n/i18n'
import { GIcon, Loading } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import PluginIcon from '@/components/plugins/PluginIcon.vue'
import { NSelect, NCheckbox, NCheckboxGroup } from 'naive-ui'
import { PLUGIN_PERMISSIONS } from '@/parts/pluginPermissions'

const props = defineProps({
  plugin: {
    type: Object,
    default: null
  },
  versions: {
    type: Array,
    default: () => []
  },
  loading: {
    type: Boolean,
    default: false
  },
  loadedInfo: {
    type: Object,
    default: null
  }
})

const isFilePlugin = computed(() => props.loadedInfo?.source_type === 'file' || props.plugin?.source_type === 'file')

const loadedStatusClasses = {
  active: 'bg-success-soft text-success-soft-text',
  error: 'bg-danger-soft text-danger-soft-text',
  updating: 'bg-warning-soft text-warning-soft-text',
  disabled: 'bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300',
}

const loadedStatus = computed(() => {
  const status = props.loadedInfo?.status || (props.loadedInfo?.enabled ? 'active' : 'disabled')

  return loadedStatusClasses[status] ? status : 'disabled'
})

const loadedStatusClass = computed(() => loadedStatusClasses[loadedStatus.value])

const emit = defineEmits(['install', 'update', 'uninstall', 'save-permissions'])

const requiredPermissions = computed(() => props.loadedInfo?.required_permissions ?? [])
const allowedPermissions = computed(() => props.loadedInfo?.allowed_permissions ?? [])
const usedPermissions = computed(() => props.loadedInfo?.used_permissions ?? [])
const missingPermissions = computed(() => props.loadedInfo?.missing_permissions ?? [])

const selectedPermissions = ref([])

// The checkboxes follow the record; a saved update flows back through
// loadedInfo, an unsaved edit is reset when another plugin is opened.
watch(allowedPermissions, (allowed) => {
  selectedPermissions.value = [...allowed]
}, { immediate: true })

const permissionsChanged = computed(() => {
  const current = [...selectedPermissions.value].sort()
  const saved = [...allowedPermissions.value].sort()

  return current.length !== saved.length || current.some((permission, i) => permission !== saved[i])
})

function permissionLabel(permission) {
  return trans('plugins.permission_' + permission)
}

const selectedVersion = ref(null)
const isSmallScreen = ref(window.innerWidth < 768)

const handleResize = () => {
  isSmallScreen.value = window.innerWidth < 768
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

const hasUpdate = computed(() => {
  if (!props.plugin?.installed) return false
  if (!props.plugin.latest_version) return false
  return props.plugin.installed_version !== props.plugin.latest_version
})

const requiresSubscriptionPurchase = computed(() => {
  return props.plugin?.requires_subscription && props.plugin?.has_subscription !== true
})

function openSubscriptionUrl() {
  if (props.plugin?.subscription_url) {
    window.open(props.plugin.subscription_url, '_blank')
  }
}

const versionOptions = computed(() => {
  return props.versions.map(v => ({
    label: v.version,
    value: v.version
  }))
})

const selectedVersionData = computed(() => {
  if (!selectedVersion.value) return null
  return props.versions.find(v => v.version === selectedVersion.value)
})

// Set default selected version to latest
watch([() => props.versions, () => props.plugin], ([versions, plugin]) => {
  if (plugin?.latest_version) {
    selectedVersion.value = plugin.latest_version
  } else if (versions?.length > 0) {
    selectedVersion.value = versions[0].version
  }
}, { immediate: true })

function renderStars(rating) {
  const fullStars = Math.floor(rating || 0)
  const hasHalf = (rating || 0) - fullStars >= 0.5
  const emptyStars = 5 - fullStars - (hasHalf ? 1 : 0)

  return '★'.repeat(fullStars) + (hasHalf ? '½' : '') + '☆'.repeat(emptyStars)
}

function formatNumber(num) {
  if (!num) return '0'
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

function formatDate(dateString) {
  if (!dateString) return ''
  const date = new Date(dateString)
  return date.toLocaleDateString()
}

function formatDateTime(dateString) {
  if (!dateString) return ''
  const date = new Date(dateString)
  return date.toLocaleString()
}
</script>
