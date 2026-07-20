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
          :scroll-x="isSmallScreen ? 460 : 920"
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

  <UploadPluginModal
      v-model:show="uploadModalVisible"
      @installed="onPluginInstalled"
  />
</template>

<script setup>
import { GBreadcrumbs, Loading, GIcon, GDataTable, GModal, GEmpty } from "@gameap/ui"
import { computed, ref, onMounted, onUnmounted, h } from "vue"
import { trans } from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import PluginIcon from "@/components/plugins/PluginIcon.vue"
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
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.plugins.index'}, 'text':trans('plugins.plugins')},
  ]
})

const activeTab = ref('installed')
const detailsModalVisible = ref(false)
const actionLoading = ref(false)
const storePage = ref(1)
const isSmallScreen = ref(window.innerWidth < 768)
const subscriptionModalVisible = ref(false)
const subscriptionPlugin = ref(null)
const uploadModalVisible = ref(false)


const handleResize = () => {
  isSmallScreen.value = window.innerWidth < 768
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

const currentLoadedInfo = computed(() => {
  if (!currentPlugin.value) return null
  return loadedPlugins.value.find(p => p.id === currentPlugin.value.id) || null
})

const installedPagination = {
  pageSize: 15,
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
            ? 'px-2 py-0.5 text-xs font-medium rounded-full bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300'
            : 'px-2 py-0.5 text-xs font-medium rounded-full bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300'
        }, row.isFilePlugin ? trans('plugins.source_file') : trans('plugins.source_store')))

        badges.push(h('span', {
          class: row.enabled
            ? 'px-2 py-0.5 text-xs font-medium rounded-full bg-lime-100 text-lime-800 dark:bg-lime-900 dark:text-lime-300'
            : 'px-2 py-0.5 text-xs font-medium rounded-full bg-stone-100 text-stone-800 dark:bg-stone-700 dark:text-stone-300'
        }, row.enabled ? trans('plugins.status_active') : trans('plugins.status_disabled')))

        if (row.hasUpdate) {
          badges.push(h('span', {
            class: 'px-2 py-0.5 text-xs font-medium rounded-full bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-300'
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
            h('span', { class: 'font-medium text-blue-600 dark:text-blue-400 hover:underline' }, row.name),
            row.summary ? h('div', { class: 'text-xs text-stone-500 dark:text-stone-400 line-clamp-2 whitespace-normal' }, row.summary) : null,
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
      width: isSmallScreen.value ? 80 : 180,
      render(row) {
        return h('div', { class: 'flex gap-1' }, [
          row.hasUpdate
              ? h(GButton, {
                color: 'blue',
                size: 'small',
                onClick: () => onShowDetailsForUpdate(row)
              }, () => [h(GIcon, { name: 'sync' })])
              : null,
          h(GButton, {
            color: 'red',
            size: 'small',
            onClick: () => onClickUninstall(row)
          }, () => isSmallScreen.value
              ? [h(GIcon, { name: 'close' })]
              : [h(GIcon, { name: 'close', class: 'mr-1' }), trans('plugins.uninstall')]),
        ])
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
            h('div', { class: 'flex items-center gap-2' }, [
              h('span', { class: 'font-medium text-blue-600 dark:text-blue-400 hover:underline' }, row.name),
              row.requires_subscription
                  ? h(GIcon, { name: 'star', class: 'text-yellow-500' })
                  : null,
              !isSmallScreen.value && row.installed
                  ? h('span', { class: 'px-2 py-0.5 text-xs font-medium rounded-full bg-lime-100 text-lime-800 dark:bg-lime-900 dark:text-lime-300 whitespace-nowrap' }, trans('plugins.already_installed'))
                  : null
            ]),
            row.summary ? h('div', { class: 'text-xs text-stone-500 dark:text-stone-400 line-clamp-2 whitespace-normal' }, row.summary) : null,
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
      width: isSmallScreen.value ? 80 : 150,
      render(row) {
        if (row.installed) {
          return isSmallScreen.value
              ? h(GButton, {
                color: 'gray',
                size: 'small',
                disabled: true,
              }, () => [h(GIcon, { name: 'check' })])
              : h(GButton, {
                color: 'gray',
                size: 'small',
                disabled: true,
              }, () => trans('plugins.already_installed'))
        }

        if (requiresSubscriptionPurchase(row)) {
          return isSmallScreen.value
              ? h(GButton, {
                color: 'orange',
                size: 'small',
                onClick: () => showSubscriptionModal(row)
              }, () => [h(GIcon, { name: 'star' })])
              : h(GButton, {
                color: 'orange',
                size: 'small',
                onClick: () => showSubscriptionModal(row)
              }, () => [h(GIcon, { name: 'star', class: 'mr-1' }), trans('plugins.purchase')])
        }

        return isSmallScreen.value
            ? h(GButton, {
              color: 'blue',
              size: 'small',
              onClick: () => onShowDetailsForInstall(row.id)
            }, () => [h(GIcon, { name: 'download' })])
            : h(GButton, {
              color: 'blue',
              size: 'small',
              onClick: () => onShowDetailsForInstall(row.id)
            }, () => [h(GIcon, { name: 'download', class: 'mr-1' }), trans('plugins.install')])
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
