import { computed } from 'vue'
import { usePluginsStore } from '@/store/plugins'
import { useServerStore } from '@/store/server'

const MAX_EDIT_FILE_SIZE = 1048576 // 1MB in bytes

/**
 * Check if a file exceeds the maximum editable size.
 * @param {Object} file - File object from file manager
 * @returns {boolean} True if file is too large for editing
 */
export function isFileTooLarge(file) {
    return file.size > MAX_EDIT_FILE_SIZE
}

/**
 * Composable for plugin file editor functionality in file manager.
 * Provides access to registered file editors and matching logic.
 */
export function useFileEditors() {
    const pluginsStore = usePluginsStore()
    const serverStore = useServerStore()

    const gameCode = computed(() => serverStore.server?.game_id || null)
    const gameName = computed(() => serverStore.server?.game?.name || null)

    /**
     * Get file info object from a file item.
     * @param {Object} file - File object from file manager
     * @returns {Object} File info object with fileName, filePath, extension
     */
    function getFileInfo(file) {
        return {
            fileName: file.basename,
            filePath: file.path,
            extension: file.extension
        }
    }

    /**
     * Get all matching editors for a file.
     * @param {Object} file - File object from file manager
     * @returns {Array} Sorted array of matching editors (most specific first)
     */
    function getMatchingEditors(file) {
        const fileInfo = getFileInfo(file)
        return pluginsStore.getMatchingEditors(fileInfo, {
            gameCode: gameCode.value,
            gameName: gameName.value
        })
    }

    /**
     * Get the default (most specific) editor for a file.
     * @param {Object} file - File object from file manager
     * @returns {Object|null} The default editor or null if no custom editor matches
     */
    function getDefaultEditor(file) {
        const fileInfo = getFileInfo(file)
        return pluginsStore.getDefaultEditor(fileInfo, {
            gameCode: gameCode.value,
            gameName: gameName.value
        })
    }

    /**
     * Check if a file has any custom editors available.
     * @param {Object} file - File object from file manager
     * @returns {boolean} True if custom editors are available
     */
    function hasCustomEditors(file) {
        return getMatchingEditors(file).length > 0
    }

    /**
     * Check if a file can be edited with plugin editors.
     * @param {Object} file - File object from file manager
     * @returns {boolean} True if file can be edited (has editors and not too large)
     */
    function canEditWithPlugin(file) {
        return !isFileTooLarge(file) && hasCustomEditors(file)
    }

    return {
        gameCode,
        gameName,
        getFileInfo,
        getMatchingEditors,
        getDefaultEditor,
        hasCustomEditors,
        canEditWithPlugin,
        isFileTooLarge
    }
}
