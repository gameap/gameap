<template>
    <div class="fm flex flex-col" v-bind:class="{ 'fm-full-screen': fullScreen }">
        <navbar-block />
        <div class="fm-body flex min-h-0">
            <context-menu />
            <modal-block />
            <manager class="relative flex-grow max-w-full flex-1 h-full" manager="left" />
        </div>
        <progress-block />
        <info-block />
        <transition name="fade">
            <div
                v-if="dropOver"
                class="absolute inset-0 z-50 flex items-center justify-center pointer-events-none bg-scrim border-4 border-dashed border-stone-700 dark:border-stone-300 rounded-md"
            >
                <div class="bg-white dark:bg-stone-800 px-8 py-6 rounded-lg shadow-2xl flex flex-col items-center gap-3">
                    <GIcon name="upload" class="text-5xl text-stone-700 dark:text-stone-200" />
                    <p class="text-lg font-semibold text-stone-700 dark:text-stone-200">
                        {{ lang.modal.upload.dropOverlay }}
                    </p>
                </div>
            </div>
        </transition>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { GIcon } from '@gameap/ui'
import HTTP from './http/axios.js'
import EventBus from './emitter.js'
import { errorNotification, notification } from '@/parts/dialogs.js'
import { useFileManagerStore } from './stores/useFileManagerStore.js'
import { useSettingsStore } from './stores/useSettingsStore.js'
import { useMessagesStore } from './stores/useMessagesStore.js'
import { useModalStore } from './stores/useModalStore.js'
import { useTranslate } from './composables/useTranslate.js'
import { useWindowDropZone } from './composables/useDropZone.js'

import NavbarBlock from './components/blocks/NavbarBlock.vue'
import Manager from './components/manager/Manager.vue'
import ModalBlock from './components/modals/ModalBlock.vue'
import InfoBlock from './components/blocks/InfoBlock.vue'
import ProgressBlock from './components/blocks/ProgressBlock.vue'
import ContextMenu from './components/blocks/ContextMenu.vue'

const props = defineProps({
    settings: {
        type: Object,
        default() {
            return {}
        },
    },
})

const fm = useFileManagerStore()
const settings = useSettingsStore()
const messages = useMessagesStore()
const modal = useModalStore()
const { lang } = useTranslate()

const { isOver: dropOver } = useWindowDropZone({
    isActive: () => !modal.showModal,
    onDrop: ({ files, emptyDirs }) => {
        messages.clearUploadProgress()
        modal.setModalState({
            show: true,
            modalName: 'UploadModal',
            payload: { entries: files, emptyDirs },
        })
    },
})

const interceptorIndex = ref({
    request: null,
    response: null,
})

// Computed
const fullScreen = computed(() => fm.fullScreen)

function isEditableTarget(target) {
    if (!target) return false
    const tag = target.tagName
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
    if (target.isContentEditable) return true

    return false
}

function handleGlobalKey(event) {
    if (modal.showModal) return
    if (isEditableTarget(event.target)) return

    const meta = event.ctrlKey || event.metaKey
    const key = event.key

    if (meta && (key === 'a' || key === 'A')) {
        event.preventDefault()
        fm.selectAllVisible(fm.activeManager)

        return
    }

    if (key === 'Escape') {
        if (fm.getSelectedCount(fm.activeManager) > 0) {
            event.preventDefault()
            fm.clearSelection(fm.activeManager)
        }

        return
    }

    if (key === 'Delete' && fm.getSelectedCount(fm.activeManager) > 0) {
        event.preventDefault()
        modal.setModalState({ modalName: 'DeleteModal', show: true })

        return
    }

    if (key === 'F5') {
        event.preventDefault()
        fm.refreshAll()
    }
}

// Methods
function setAxiosConfig() {
    HTTP.defaults.baseURL = settings.baseUrl

    Object.keys(settings.headers).forEach((key) => {
        HTTP.defaults.headers.common[key] = settings.headers[key]
    })
}

function requestInterceptor() {
    interceptorIndex.value.request = HTTP.interceptors.request.use(
        (config) => {
            messages.addLoading()
            return config
        },
        (error) => {
            messages.subtractLoading()
            return Promise.reject(error)
        }
    )
}

function responseInterceptor() {
    interceptorIndex.value.response = HTTP.interceptors.response.use(
        (response) => {
            messages.subtractLoading()

            const silent = response.config && response.config.silent

            if (!silent && Object.prototype.hasOwnProperty.call(response.data, 'result')) {
                if (response.data.result.message) {
                    const messageText = Object.prototype.hasOwnProperty.call(
                        lang.value.response,
                        response.data.result.message
                    )
                        ? lang.value.response[response.data.result.message]
                        : response.data.result.message

                    const notificationType = response.data.result.status === 'success' ? 'success' : 'info'
                    notification({
                        content: messageText,
                        type: notificationType,
                    })

                    messages.setActionResult({
                        status: response.data.result.status,
                        message: messageText,
                    })
                }
            }

            return response
        },
        (error) => {
            messages.subtractLoading()

            const silent = error.config && (error.config.silent || error.config.silentError)
            const aborted = error.code === 'ERR_CANCELED' || error.message === 'canceled'

            const errorMessage = {
                status: 0,
                message: '',
            }

            if (error.response) {
                errorMessage.status = error.response.status

                if (error.response.data && error.response.data.message) {
                    errorMessage.message = Object.prototype.hasOwnProperty.call(
                        lang.value.response,
                        error.response.data.message
                    )
                        ? lang.value.response[error.response.data.message]
                        : error.response.data.message
                } else {
                    errorMessage.message = error.response.statusText || `HTTP ${error.response.status}`
                }
            } else if (error.request) {
                errorMessage.status = error.request.status
                errorMessage.message = error.request.statusText || 'Network error'
            } else {
                errorMessage.message = error.message
            }

            if (!silent && !aborted) {
                messages.setError(errorMessage)
                errorNotification(errorMessage.message)
            }

            return Promise.reject(error)
        }
    )
}

watch(
    () => props.settings && props.settings.serverName,
    (name) => {
        settings.manualSettings({ serverName: name || '' })
    },
)

// Lifecycle
onMounted(() => {
    settings.manualSettings(props.settings)
    settings.initAxiosSettings()
    setAxiosConfig()
    requestInterceptor()
    responseInterceptor()
    fm.initializeApp()
    document.addEventListener('keydown', handleGlobalKey)
})

onUnmounted(() => {
    document.removeEventListener('keydown', handleGlobalKey)
    fm.resetState()
    EventBus.all.clear()
    HTTP.interceptors.request.eject(interceptorIndex.value.request)
    HTTP.interceptors.response.eject(interceptorIndex.value.response)
})
</script>

<style lang="scss">
.fm {
    position: relative;
    height: 100%;
    padding: 1rem;

    .fm-body {
        flex: 1 1 0;
        min-height: 0;
        overflow: hidden;
        position: relative;
    }

    .unselectable {
        user-select: none;
    }
}

.fm-error {
    @apply text-red-500 dark:text-red-400 bg-red-100 dark:bg-red-900 border-red-500 dark:border-red-400;
}

.fm-danger {
    @apply text-white bg-red-100 dark:bg-red-900 border-red-500 dark:border-red-400;
}

.fm-warning {
  @apply text-orange-500 dark:text-orange-400 bg-orange-100 dark:bg-orange-900 border-orange-500 dark:border-orange-400;
}

.fm-success {
  @apply text-white bg-success dark:bg-lime-800 border-success dark:border-lime-400;
}

.fm-info {
  @apply text-white bg-stone-600 dark:bg-stone-700 border-stone-500 dark:border-stone-400;
}

.fm.fm-full-screen {
    width: 100%;
    height: 100%;
    padding-bottom: 0;
}
</style>
