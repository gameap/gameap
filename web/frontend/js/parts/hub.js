import {getCurrentLanguage} from '@/i18n/i18n'

const HUB_URLS = {
    ru: 'https://hub.gameap.ru/',
    de: 'https://hub.gameap.com/de/',
    es: 'https://hub.gameap.com/es/',
}

// Locales the hub has no version for (English included, plugins may add more)
// get the international site.
const DEFAULT_HUB_URL = 'https://hub.gameap.com/'

export function hubUrl(lang = getCurrentLanguage()) {
    return HUB_URLS[lang] ?? DEFAULT_HUB_URL
}

export function hubHost(lang = getCurrentLanguage()) {
    return new URL(hubUrl(lang)).host
}
