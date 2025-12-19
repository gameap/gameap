import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

import ru from '../lang/ru.js'
import en from '../lang/en.js'
import ar from '../lang/ar.js'
import sr from '../lang/sr.js'
import cs from '../lang/cs.js'
import de from '../lang/de.js'
import es from '../lang/es.js'
import nl from '../lang/nl.js'
import zh_CN from '../lang/zh_CN.js'
import fa from '../lang/fa.js'
import it from '../lang/it.js'
import tr from '../lang/tr.js'
import fr from '../lang/fr.js'
import pt_BR from '../lang/pt_BR.js'
import zh_TW from '../lang/zh_TW.js'
import pl from '../lang/pl.js'
import hu from '../lang/hu.js'

export const useSettingsStore = defineStore('fm-settings', () => {
    const acl = ref(null)
    const version = ref('4.0.0-dev2')
    const headers = ref({})
    const baseUrl = ref(null)

    /**
     * File manager windows configuration
     * 1 - only one file manager window
     * 2 - one file manager window with directories tree module
     * 3 - two file manager windows
     */
    const windowsConfig = ref(null)

    const lang = ref('en')

    const translations = ref({
        ru: Object.freeze(ru),
        en: Object.freeze(en),
        ar: Object.freeze(ar),
        sr: Object.freeze(sr),
        cs: Object.freeze(cs),
        de: Object.freeze(de),
        es: Object.freeze(es),
        nl: Object.freeze(nl),
        'zh-CN': Object.freeze(zh_CN),
        fa: Object.freeze(fa),
        it: Object.freeze(it),
        tr: Object.freeze(tr),
        fr: Object.freeze(fr),
        'pt-BR': Object.freeze(pt_BR),
        'zh-TW': Object.freeze(zh_TW),
        pl: Object.freeze(pl),
        hu: Object.freeze(hu),
    })

    const hiddenFiles = ref(false)

    const contextMenu = ref([
        [
            { name: 'open', icon: 'fa-regular fa-folder-open' },
            { name: 'audioPlay', icon: 'fa-regular fa-play' },
            { name: 'videoPlay', icon: 'fa-regular fa-play' },
            { name: 'view', icon: 'fa-solid fa-eye' },
            { name: 'edit', icon: 'fa-solid fa-pen' },
            { name: 'select', icon: 'fa-solid fa-check' },
            { name: 'download', icon: 'fa-solid fa-download' },
        ],
        [
            { name: 'copy', icon: 'fa-regular fa-copy' },
            { name: 'cut', icon: 'fa-solid fa-scissors' },
            { name: 'rename', icon: 'fa-regular fa-pen-to-square' },
            { name: 'paste', icon: 'fa-regular fa-paste' },
        ],
        [
            { name: 'delete', icon: 'fa-regular fa-trash-can text-danger' },
        ],
        [
            { name: 'properties', icon: 'fa-regular fa-rectangle-list' },
        ],
    ])

    const imageExtensions = ref(['png', 'jpg', 'jpeg', 'gif', 'webp'])
    const cropExtensions = ref(['png', 'jpg', 'jpeg', 'webp'])
    const audioExtensions = ref(['ogg', 'mp3', 'aac', 'wav'])
    const videoExtensions = ref(['webm', 'mp4'])

    const textExtensions = ref({
        sh: 'text/x-sh',
        css: 'text/css',
        less: 'text/x-less',
        sass: 'text/x-sass',
        scss: 'text/x-scss',
        html: 'text/html',
        js: 'text/javascript',
        ts: 'text/typescript',
        vue: 'text/x-vue',
        htaccess: 'text/plain',
        env: 'text/plain',
        txt: 'text/plain',
        log: 'text/plain',
        ini: 'text/x-ini',
        xml: 'application/xml',
        cfg: 'text/plain',
        md: 'text/x-markdown',
        java: 'text/x-java',
        c: 'text/x-csrc',
        cpp: 'text/x-c++src',
        cs: 'text/x-csharp',
        scl: 'text/x-scala',
        php: 'application/x-httpd-php',
        sql: 'text/x-sql',
        pl: 'text/x-perl',
        py: 'text/x-python',
        lua: 'text/x-lua',
        swift: 'text/x-swift',
        rb: 'text/x-ruby',
        go: 'text/x-go',
        yaml: 'text/x-yaml',
        json: 'application/json',
        properties: 'text/plain',
    })

    // Getters
    const authHeader = computed(() => Object.prototype.hasOwnProperty.call(headers.value, 'Authorization'))

    // Actions
    function manualSettings(data) {
        if (Object.prototype.hasOwnProperty.call(data, 'headers')) {
            headers.value = data.headers
        }
        if (Object.prototype.hasOwnProperty.call(data, 'baseUrl')) {
            baseUrl.value = data.baseUrl
        }
        if (Object.prototype.hasOwnProperty.call(data, 'windowsConfig')) {
            windowsConfig.value = data.windowsConfig
        }
        if (Object.prototype.hasOwnProperty.call(data, 'lang')) {
            lang.value = data.lang
        }
        if (Object.prototype.hasOwnProperty.call(data, 'translation')) {
            translations.value[data.translation.name] = Object.freeze(data.translation.content)
        }
    }

    function initAxiosSettings() {
        if (!baseUrl.value) {
            if (import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL) {
                baseUrl.value = import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL
            } else if (import.meta.env.VITE_LFM_BASE_URL) {
                baseUrl.value = import.meta.env.VITE_LFM_BASE_URL
            } else {
                let url = `${window.location.protocol}//${window.location.hostname}`
                if (window.location.port.length) {
                    url += `:${window.location.port}/api/file-manager/`
                } else {
                    url += '/api/file-manager/'
                }
                baseUrl.value = url
            }
        }
    }

    function initSettings(data) {
        if (!lang.value) lang.value = data.lang
        if (!windowsConfig.value) windowsConfig.value = data.windowsConfig
        acl.value = data.acl
        hiddenFiles.value = data.hiddenFiles
    }

    function toggleHiddenFiles() {
        hiddenFiles.value = !hiddenFiles.value
    }

    return {
        // State
        acl,
        version,
        headers,
        baseUrl,
        windowsConfig,
        lang,
        translations,
        hiddenFiles,
        contextMenu,
        imageExtensions,
        cropExtensions,
        audioExtensions,
        videoExtensions,
        textExtensions,
        // Getters
        authHeader,
        // Actions
        manualSettings,
        initAxiosSettings,
        initSettings,
        toggleHiddenFiles,
    }
})
