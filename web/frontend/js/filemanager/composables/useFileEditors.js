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
 * Whether the editor loads the file itself instead of being handed its
 * content. The size cap exists because the modal downloads the file before
 * mounting the editor; an editor that downloads nothing is not bound by it.
 * @param {Object} editor - Registered editor definition
 * @returns {boolean} True if the modal hands the editor no content
 */
export function loadsOwnContent(editor) {
    return editor?.contentType === 'none'
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
     * Whether the current user satisfies an editor's permission check. Same
     * shape the server tabs use, so a plugin declares access the one way.
     * @param {Object} editor - Registered editor definition
     * @returns {boolean} True when the editor may be offered
     */
    function isPermitted(editor) {
        const check = editor.checkPermission
        if (!check) return true
        if (check.type === 'hasServerPermissions') {
            return check.permissions.every(perm => serverStore.abilities[perm] === true)
        }
        return true
    }

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
        const permitted = pluginsStore.getMatchingEditors(fileInfo, {
            gameCode: gameCode.value,
            gameName: gameName.value
        }).filter(item => isPermitted(item.editor))

        // The store marks the default before permissions are known, so the one
        // it picked may be the one just filtered out. The default is the most
        // specific editor a double click can actually open.
        const preferred = permitted.find(item => !item.editor.contextMenuOnly)

        return permitted.map(item => ({ ...item, isDefault: item === preferred }))
    }

    /**
     * Get the default (most specific) editor for a file.
     * @param {Object} file - File object from file manager
     * @returns {Object|null} The default editor or null if no custom editor matches
     */
    function getDefaultEditor(file) {
        return getMatchingEditors(file).find(item => item.isDefault) ?? null
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
        if (!isFileTooLarge(file)) {
            return hasCustomEditors(file)
        }
        return getMatchingEditors(file).some(item => loadsOwnContent(item.editor))
    }

    return {
        gameCode,
        gameName,
        getFileInfo,
        getMatchingEditors,
        getDefaultEditor,
        hasCustomEditors,
        canEditWithPlugin,
        isFileTooLarge,
        loadsOwnContent
    }
}
