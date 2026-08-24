<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <n-tabs v-model:value="activeTab" type="line" animated @update:value="onTabChange">
    <n-tab-pane name="installed" :tab="trans('plugins.installed')">
      <div class="flex mb-4">
        <GButton color="blue" @click="showUploadModal">
          <GIcon name="upload" class="mr-1" />
          {{ trans('plugins.upload') }}
        </GButton>
      </div>

      <GDataTable
          :columns="installedColumns"
          :data="enrichedInstalledPlugins"
          :loading="loading"
          :pagination="installedPagination"
          :scroll-x="isSmallScreen ? 460 : 1140"
      >
        <template #loading>
          <Loading />
        </template>
        <template #empty>
          <GEmpty :description="trans('plugins.no_plugins')"></GEmpty>
        </template>
      </GDataTable>
    </n-tab-pane>

    <n-tab-pane name="store" :tab="trans('plugins.store')">
      <GDataTable
          :columns="storeColumns"
          :data="plugins"
          :loading="loading"
          :pagination="false"
          :scroll-x="isSmallScreen ? 460 : 920"
      >
        <template #loading>
          <Loading />
        </template>
        <template #empty>
          <GEmpty :description="trans('plugins.no_plugins')"></GEmpty>
        </template>
      </GDataTable>

      <div class="flex justify-center mt-4" v-if="lastPage > 1">
        <n-pagination
            v-model:page="storePage"
            :page-count="lastPage"
            @update:page="onStorePageChange"
        />
      </div>
    </n-tab-pane>
  </n-tabs>

  <GModal
      v-model:show="detailsModalVisible"
      :title="currentPlugin?.name || ''"
      style="width: 900px; max-width: 90vw;"
  >
    <n-spin :show="actionLoading">
      <PluginDetailsModal
          v-if="currentPlugin"
          :plugin="currentPlugin"
          :versions="currentPluginVersions"
          :loading="loading"
          :loaded-info="currentLoadedInfo"
          @install="onInstall"
          @update="onUpdate"
          @uninstall="onUninstall"
          @close="closeDetailsModal"
      />
    </n-spin>
  </GModal>

  <SubscriptionModal
      v-model:show="subscriptionModalVisible"
      :plugin="subscriptionPlugin"
  />

  <PluginPermissionsModal
      v-model:show="permissionsModalVisible"
      :plugin="permissionsPlugin"
  />

  <UploadPluginModal
      v-model:show="uploadModalVisible"
      @installed="onPluginInstalled"
  />
</template>

<script setup>
import { GBreadcrumbs, Loading, GIcon, GDataTable, GModal, GEmpty } from "@gameap/ui"
import { computed, ref, onMounted, h } from "vue"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import PluginIcon from "@/components/plugins/PluginIcon.vue"
import { useIsSmallScreen } from "@/composables/useIsSmallScreen"
import { usePluginStoreStore } from "@/store/pluginStore"
import { errorNotification, notification } from "@/parts/dialogs"
import {
  NTabs,
  NTabPane,
  NPagination,
  NSpin,
} from "naive-ui"
import { storeToRefs } from "pinia"
import PluginDetailsModal from "./forms/PluginDetailsModal.vue"
import SubscriptionModal from "./forms/SubscriptionModal.vue"
import PluginPermissionsModal from "./forms/PluginPermissionsModal.vue"
import UploadPluginModal from "./forms/UploadPluginModal.vue"

const pluginStore = usePluginStoreStore()

const {
  plugins,
  lastPage,
  currentPlugin,
  currentPluginVersions,
  loading,
  enrichedInstalledPlugins,
  loadedPlugins,
} = storeToRefs(pluginStore)

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'admin.plugins.index'}, 'text':trans('plugins.plugins')},
  ]
})

const activeTab = ref('installed')
const detailsModalVisible = ref(false)
const actionLoading = ref(false)
const storePage = ref(1)
const subscriptionModalVisible = ref(false)
const subscriptionPlugin = ref(null)
const uploadModalVisible = ref(false)
const permissionsModalVisible = ref(false)
const permissionsPlugin = ref(null)

// Columns are dropped below md, action labels below lg.
const isSmallScreen = useIsSmallScreen()
const isCompactActions = useIsSmallScreen(1024)

const currentLoadedInfo = computed(() => {
  if (!currentPlugin.value) return null
  return loadedPlugins.value.find(p => p.id === currentPlugin.value.id) || null
})

const installedPagination = {
  pageSize: 15,
}

function renderActionButton(color, iconName, label, onClick, extra = {}) {
  return h(GButton, { color, size: 'small', class: 'mr-0.5', onClick, ...extra }, {
    default: () => [
      h(GIcon, { name: iconName }),
      h('span', { class: 'hidden lg:inline ml-1' }, label),
    ],
  })
}

const createInstalledColumns = () => {
  return [
    {
      title: trans('plugins.name'),
      key: 'name',
      render(row) {
        const badges = []

        badges.push(h('span', {
          class: row.isFilePlugin
            ? 'px-2 py-0.5 text-xs font-medium rounded-full bg-info-soft text-info-soft-text'
            : 'px-2 py-0.5 text-xs font-medium rounded-full bg-success-soft text-success-soft-text'
        }, row.isFilePlugin ? trans('plugins.source_file') : trans('plugins.source_store')))

        const status = statusBadge(row.status)
        badges.push(h('span', {
          class: 'px-2 py-0.5 text-xs font-medium rounded-full ' + status.class,
          title: row.status === 'error' && row.error ? row.error : undefined
        }, status.label))

        if (!row.loaded && row.status !== 'error') {
          badges.push(h('span', {
            class: 'px-2 py-0.5 text-xs font-medium rounded-full bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300'
          }, trans('plugins.not_loaded')))
        }

        if (row.hasUpdate) {
          badges.push(h('span', {
            class: 'px-2 py-0.5 text-xs font-medium rounded-full bg-warning-soft text-warning-soft-text'
          }, trans('plugins.update_available')))
        }

        if (!isSmallScreen.value && row.labels?.length > 0) {
          row.labels.forEach(label => {
            badges.push(h('span', {
              class: 'px-2 py-0.5 text-xs font-medium rounded-full' + (!label.color ? ' bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300' : ''),
              style: label.color ? { backgroundColor: label.color, color: '#fff' } : {}
            }, label.name))
          })
        }

        return h('div', {
          class: 'flex items-center gap-2 cursor-pointer hover:opacity-80',
          onClick: () => onShowDetails(row)
        }, [
          h('div', { class: 'shrink-0' }, [h(PluginIcon, { plugin: row })]),
          h('div', { class: 'flex flex-col min-w-0' }, [
            h('span', { class: 'font-medium text-info hover:underline break-words' }, row.name),
            row.summary ? h('div', { class: 'text-xs text-stone-500 dark:text-stone-400 line-clamp-2 whitespace-normal break-words' }, row.summary) : null,
            row.status === 'error' && row.error
              ? h('div', { class: 'text-xs text-danger line-clamp-2 whitespace-normal break-words' }, row.error)
              : null,
            badges.length > 0 ? h('div', { class: 'flex gap-1 mt-1 flex-wrap' }, badges) : null
          ])
        ])
      },
    },
    {
      title: trans('plugins.category'),
      key: 'category',
      width: 120,
      render(row) {
        return row.category?.name || '-'
      }
    },
    {
      title: trans('plugins.rating'),
      key: 'rating_avg',
      width: 140,
      render(row) {
        if (row.isFilePlugin) {
          return '-'
        }
        return h('div', { class: 'flex items-center gap-1' }, [
          h('span', { class: 'text-orange-500' }, renderStars(row.rating_avg)),
          h('span', { class: 'text-sm text-stone-500' }, `(${row.rating_count || 0})`)
        ])
      }
    },
    {
      title: trans('plugins.downloads'),
      key: 'download_count',
      width: 100,
      render(row) {
        if (row.isFilePlugin) {
          return '-'
        }
        return formatNumber(row.download_count)
      }
    },
    {
      title: trans('plugins.version'),
      key: 'installed_version',
      width: 130,
      render(row) {
        if (row.hasUpdate) {
          return h('span', {}, [
            row.installed_version,
            h('span', { class: 'text-stone-400 mx-1' }, '→'),
            h('span', { class: 'text-orange-500 font-medium' }, row.latest_version)
          ])
        }
        return row.installed_version
      }
    },
    {
      title: trans('main.actions'),
      key: 'actions',
      align: 'right',
      // Wide enough for the longest locale (de) with the update button shown.
      width: isCompactActions.value ? 160 : 460,
      render(row) {
        // null when the instance that answered has not loaded the plugin: it
        // cannot tell which permissions the module exercises, so the button
        // stays neutral instead of claiming everything is granted.
        const missingPermissions = row.missing_permissions
        const permissionsUnknown = missingPermissions === null

        return [
          renderActionButton('black', 'refresh', trans('plugins.reload'), () => onReload(row), {
            disabled: row.status === 'updating',
          }),
          row.hasUpdate
              ? renderActionButton('blue', 'sync', trans('plugins.update'), () => onShowDetailsForUpdate(row))
              : null,
          renderActionButton(
              permissionsUnknown
                  ? 'white'
                  : (missingPermissions.length > 0 ? 'orange' : 'green'),
              'key',
              trans('plugins.permissions'),
              () => onShowPermissions(row),
              {
                title: permissionsUnknown
                    ? trans('plugins.permissions_unknown')
                    : (missingPermissions.length > 0
                        ? missingPermissions.map(permission => trans('plugins.permission_' + permission)).join(', ')
                        : undefined),
                'data-testid': `plugin-row-permissions-${row.id}`,
              },
          ),
          renderActionButton('red', 'delete', trans('plugins.uninstall'), () => onClickUninstall(row), {
            class: '',
          }),
        ]
      },
    }
  ]
}

const createStoreColumns = () => {
  return [
    {
      title: trans('plugins.name'),
      key: 'name',
      render(row) {
        return h('div', {
          class: 'flex items-center gap-2 cursor-pointer hover:opacity-80',
          onClick: () => onShowDetailsById(row.id)
        }, [
          h('div', { class: 'shrink-0' }, [h(PluginIcon, { plugin: row })]),
          h('div', { class: 'flex flex-col min-w-0' }, [
            h('div', { class: 'flex items-center gap-2 min-w-0' }, [
              h('span', { class: 'font-medium text-info hover:underline break-words min-w-0' }, row.name),
              row.requires_subscription
                  ? h(GIcon, { name: 'star', class: 'text-yellow-500' })
                  : null,
              !isSmallScreen.value && row.installed
                  ? h('span', { class: 'px-2 py-0.5 text-xs font-medium rounded-full bg-success-soft text-success-soft-text whitespace-nowrap' }, trans('plugins.already_installed'))
                  : null
            ]),
            row.summary ? h('div', { class: 'text-xs text-stone-500 dark:text-stone-400 line-clamp-2 whitespace-normal break-words' }, row.summary) : null,
            !isSmallScreen.value && row.labels?.length > 0
                ? h('div', { class: 'flex gap-1 mt-1 flex-wrap' },
                    row.labels.map(label =>
                        h('span', {
                          class: 'px-2 py-0.5 text-xs font-medium rounded-full' + (!label.color ? ' bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300' : ''),
                          style: label.color ? { backgroundColor: label.color, color: '#fff' } : {}
                        }, label.name)
                    )
                )
                : null
          ])
        ])
      },
    },
    {
      title: trans('plugins.category'),
      key: 'category',
      width: 120,
      render(row) {
        return row.category?.name || ''
      }
    },
    {
      title: trans('plugins.rating'),
      key: 'rating_avg',
      width: 140,
      render(row) {
        return h('div', { class: 'flex items-center gap-1' }, [
          h('span', { class: 'text-orange-500' }, renderStars(row.rating_avg)),
          h('span', { class: 'text-sm text-stone-500' }, `(${row.rating_count || 0})`)
        ])
      }
    },
    {
      title: trans('plugins.downloads'),
      key: 'download_count',
      width: 100,
      render(row) {
        return formatNumber(row.download_count)
      }
    },
    {
      title: trans('plugins.version'),
      key: 'latest_version',
      width: 100,
    },
    {
      title: trans('main.actions'),
      key: 'actions',
      align: 'right',
      width: isCompactActions.value ? 80 : 170,
      render(row) {
        if (row.installed) {
          return renderActionButton('white', 'check', trans('plugins.already_installed'), null, {
            disabled: true,
            class: '',
          })
        }

        if (requiresSubscriptionPurchase(row)) {
          return renderActionButton('orange', 'star', trans('plugins.purchase'), () => showSubscriptionModal(row), {
            class: '',
          })
        }

        return renderActionButton('blue', 'download', trans('plugins.install'), () => onShowDetailsForInstall(row.id), {
          class: '',
        })
      },
    }
  ]
}

const installedColumns = computed(() => {
  const cols = createInstalledColumns()
  if (isSmallScreen.value) {
    return cols.filter(col => !['installed_version', 'download_count', 'category', 'rating_avg'].includes(col.key))
  }
  return cols
})

const storeColumns = computed(() => {
  const cols = createStoreColumns()
  if (isSmallScreen.value) {
    return cols.filter(col => !['latest_version', 'download_count', 'category', 'rating_avg'].includes(col.key))
  }
  return cols
})

const statusBadgeClasses = {
  active: 'bg-success-soft text-success-soft-text',
  error: 'bg-danger-soft text-danger-soft-text',
  updating: 'bg-warning-soft text-warning-soft-text',
  disabled: 'bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300',
}

function statusBadge(status) {
  const known = statusBadgeClasses[status] ? status : 'disabled'

  return {
    class: statusBadgeClasses[known],
    label: trans('plugins.status_' + known),
  }
}

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

function onShowPermissions(row) {
  permissionsPlugin.value = row
  permissionsModalVisible.value = true
}

function showSubscriptionModal(plugin) {
  subscriptionPlugin.value = plugin
  subscriptionModalVisible.value = true
}

function requiresSubscriptionPurchase(row) {
  return row.requires_subscription && row.has_subscription !== true && !row.installed
}

function onTabChange(tab) {
  if (tab === 'store' && plugins.value.length === 0) {
    fetchStorePlugins()
  }
}

function onStorePageChange(page) {
  storePage.value = page
  fetchStorePlugins()
}

function fetchStorePlugins() {
  pluginStore.fetchPlugins({
    page: storePage.value,
  }).catch(errorNotification)
}

function onShowDetails(row) {
  if (row.isStorePlugin) {
    pluginStore.fetchPluginDetails(row.id).catch(errorNotification)
    pluginStore.fetchPluginVersions(row.id).catch(errorNotification)
  } else {
    pluginStore.setCurrentPluginFromLoaded(row)
  }
  detailsModalVisible.value = true
}

function onShowDetailsById(id) {
  pluginStore.fetchPluginDetails(id).catch(errorNotification)
  pluginStore.fetchPluginVersions(id).catch(errorNotification)
  detailsModalVisible.value = true
}

function onShowDetailsForInstall(id) {
  onShowDetailsById(id)
}

function onShowDetailsForUpdate(row) {
  onShowDetails(row)
}

function closeDetailsModal() {
  detailsModalVisible.value = false
  pluginStore.clearCurrentPlugin()
}

function onInstall(version) {
  if (!currentPlugin.value) return
  if (actionLoading.value) return

  actionLoading.value = true
  pluginStore.installPlugin(currentPlugin.value.id, version)
      .then(() => {
        closeDetailsModal()
        notification({
          content: trans('plugins.install_success_msg'),
          type: 'success'
        }, () => window.location.reload())
      })
      .catch(errorNotification)
      .finally(() => {
        actionLoading.value = false
      })
}

function onUpdate(version) {
  if (!currentPlugin.value) return
  if (actionLoading.value) return

  actionLoading.value = true
  pluginStore.updatePlugin(currentPlugin.value.id, version)
      .then(() => {
        closeDetailsModal()
        notification({
          content: trans('plugins.update_success_msg'),
          type: 'success'
        }, () => window.location.reload())
      })
      .catch(errorNotification)
      .finally(() => {
        actionLoading.value = false
      })
}

function onUninstall() {
  if (!currentPlugin.value) return

  window.$dialog.warning({
    title: trans('plugins.uninstall_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no'),
    closable: false,
    onPositiveClick: () => {
      actionLoading.value = true
      pluginStore.uninstallPlugin(currentPlugin.value.id)
          .then(() => {
            closeDetailsModal()
            notification({
              content: trans('plugins.uninstall_success_msg'),
              type: 'success'
            }, () => window.location.reload())
          })
          .catch(errorNotification)
          .finally(() => {
            actionLoading.value = false
          })
    }
  })
}

function onClickUninstall(row) {
  window.$dialog.warning({
    title: trans('plugins.uninstall_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no'),
    closable: false,
    onPositiveClick: () => {
      pluginStore.uninstallPlugin(row.id)
          .then(() => {
            notification({
              content: trans('plugins.uninstall_success_msg'),
              type: 'success'
            }, () => window.location.reload())
          })
          .catch(errorNotification)
    }
  })
}

function onReload(row) {
  pluginStore.reloadPlugin(row.id)
      .then(() => {
        notification({
          content: trans('plugins.reload_success_msg'),
          type: 'success'
        })

        return pluginStore.fetchLoadedPlugins()
      })
      .catch((error) => {
        errorNotification(error)
        pluginStore.fetchLoadedPlugins().catch(() => {})
      })
}

function refreshData() {
  pluginStore.fetchPlugins({ page: 1, perPage: 100 }).catch(errorNotification)
  pluginStore.fetchLoadedPlugins().catch(errorNotification)
}

function showUploadModal() {
  uploadModalVisible.value = true
}

function onPluginInstalled() {
  uploadModalVisible.value = false
  notification({
    content: trans('plugins.install_success_msg'),
    type: 'success'
  }, () => window.location.reload())
}

onMounted(() => {
  pluginStore.fetchPlugins({ page: 1, perPage: 100 }).catch(errorNotification)
  pluginStore.fetchLoadedPlugins().catch(errorNotification)
})
</script>
