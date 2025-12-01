/**
 * Common utilities and re-exports for convenient importing
 *
 * Usage:
 * import { axios, _, trans, errorNotification } from '@/utils/common'
 */

// HTTP client
export { default as axios } from '@/config/axios'

// Lodash utilities
export * as _ from 'lodash-es'

// i18n functions
export { trans, pluralize, changeLanguage, getCurrentLanguage, pageLanguage } from '@/i18n/i18n'

// Dialogs and notifications
export { alert, confirm, confirmAction } from '@/parts/dialogs'
export { errorNotification, notification } from '@/parts/dialogs'
