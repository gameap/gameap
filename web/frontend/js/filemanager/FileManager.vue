<template>
    <div class="fm flex flex-col" v-bind:class="{ 'fm-full-screen': fullScreen }">
        <navbar-block />
        <div class="fm-body flex min-h-0">
            <context-menu />
            <modal-block />
            <manager class="relative flex-grow max-w-full flex-1 h-full" manager="left" />
        </div>
        <info-block />
    </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import HTTP from './http/axios.js'
import EventBus from './emitter.js'
import { errorNotification, notification } from '@/parts/dialogs.js'
import { useFileManagerStore } from './stores/useFileManagerStore.js'
import { useSettingsStore } from './stores/useSettingsStore.js'
import { useMessagesStore } from './stores/useMessagesStore.js'
import { useTranslate } from './composables/useTranslate.js'

import NavbarBlock from './components/blocks/NavbarBlock.vue'
import Manager from './components/manager/Manager.vue'
import ModalBlock from './components/modals/ModalBlock.vue'
import InfoBlock from './components/blocks/InfoBlock.vue'
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
const { lang } = useTranslate()

const interceptorIndex = ref({
    request: null,
    response: null,
})

// Computed
const fullScreen = computed(() => fm.fullScreen)

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

            if (Object.prototype.hasOwnProperty.call(response.data, 'result')) {
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

            const errorMessage = {
                status: 0,
                message: '',
            }

            if (error.response) {
                errorMessage.status = error.response.status

                if (error.response.data.message) {
                    errorMessage.message = Object.prototype.hasOwnProperty.call(
                        lang.value.response,
                        error.response.data.message
                    )
                        ? lang.value.response[error.response.data.message]
                        : error.response.data.message
                } else {
                    errorMessage.message = error.response.statusText
                }
            } else if (error.request) {
                errorMessage.status = error.request.status
                errorMessage.message = error.request.statusText || 'Network error'
            } else {
                errorMessage.message = error.message
            }

            messages.setError(errorMessage)
            errorNotification(errorMessage.message)

            return Promise.reject(error)
        }
    )
}

// Lifecycle
onMounted(() => {
    settings.manualSettings(props.settings)
    settings.initAxiosSettings()
    setAxiosConfig()
    requestInterceptor()
    responseInterceptor()
    fm.initializeApp()
})

onUnmounted(() => {
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
  @apply text-white bg-lime-500 dark:bg-lime-800 border-lime-500 dark:border-lime-400;
}

.fm-info {
  @apply text-white bg-sky-500 dark:bg-sky-800 border-sky-500 dark:border-sky-400;
}

.fm.fm-full-screen {
    width: 100%;
    height: 100%;
    padding-bottom: 0;
}
</style>
