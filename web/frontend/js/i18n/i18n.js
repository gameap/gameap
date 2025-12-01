// import { createApp } from 'vue';
import plurals from './plurals';
import { get, eachRight, replace } from 'lodash-es';

const pluralForms = {
    default: (n) => (n !== 1 ? 1 : 0),
    en: (n) => (n !== 1 ? 1 : 0),
    ru: (n) =>
        n % 10 === 1 && n % 100 !== 11
            ? 0
            : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20)
                ? 1
                : 2,
};

const pluralize = (choice, choicesLength) => {
    let lang = document.documentElement.lang;

    if (!plurals.hasOwnProperty(lang)) {
        lang = 'default';
    }

    if (!plurals[lang].hasOwnProperty(choice)) {
        return choice;
    }

    const index = pluralForms[lang](choicesLength);

    return plurals[lang][choice][index] === undefined
        ? plurals[lang][choice][0]
        : plurals[lang][choice][index];
};

const trans = (string, args) => {
    let value = get(window.i18n, string);

    eachRight(args, (paramVal, paramKey) => {
        value = replace(value, `:${paramKey}`, paramVal);
    });
    return value;
};

let lang = null;
const pageLanguage = () => {
    if (lang) {
        return lang;
    }

    const htmlEl = document.getElementsByTagName('html')
    if (htmlEl.length === 0) {
        lang = navigator.language ?? 'en'
        return lang
    }

    const langAttribute = htmlEl[0].getAttribute('lang')

    if (langAttribute === null) {
        lang = navigator.language ?? 'en'
        return lang
    }

    lang = langAttribute ?? 'en'
    return lang
}

/**
 * Change the application language
 * @param {string} newLang - Language code ('en' or 'ru')
 */
const changeLanguage = (newLang) => {
    const supportedLangs = ['en', 'ru'];
    if (!supportedLangs.includes(newLang)) {
        console.error('[i18n] Unsupported language:', newLang);

        return;
    }

    // Reload the page with new language parameter
    // The server will handle setting the language via cookies or session
    // window.location.href = window.location.pathname + '?lang=' + newLang;
}

/**
 * Get current language code
 * @returns {string} Current language code
 */
const getCurrentLanguage = () => {
    return window.gameapLang || pageLanguage();
}

export {pluralize, trans, pageLanguage, changeLanguage, getCurrentLanguage}
