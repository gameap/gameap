import { useFileManagerStore } from '../stores/useFileManagerStore.js'
import { useSettingsStore } from '../stores/useSettingsStore.js'
import { useModalStore } from '../stores/useModalStore.js'
import { useHistoryStore } from '../stores/useHistoryStore.js'
import { useFileEditors, isFileTooLarge, loadsOwnContent } from './useFileEditors.js'

/**
 * Opens a file the way a double click does: routes it to the plugin editor,
 * text editor, viewer, player or PDF tab. Shared by the table (double click
 * and keyboard) and the history popover, so every entry point records the
 * open in the visit history the same way.
 */
export function useFileOpener(managerName = 'left') {
    const fm = useFileManagerStore()
    const settings = useSettingsStore()
    const modal = useModalStore()
    const history = useHistoryStore()
    const { getDefaultEditor } = useFileEditors()

    function disk() {
        return fm.getManager(managerName).selectedDisk
    }

    function noteOpened(file, recordFile = true) {
        history.noteFileOpened({
            disk: disk(),
            path: file.path,
            dirname: file.dirname,
            recordFile,
        })
    }

    // The built-in modals read fm.selectedItems[0], so the file must be the
    // selection before the modal mounts — a focused-but-unselected row would
    // otherwise open whatever was selected last.
    function openSelectionModal(file, modalName) {
        fm.changeSelected(managerName, { type: 'files', path: file.path })
        modal.setModalState({ modalName, show: true })
        noteOpened(file)
    }

    function openFile(file) {
        const { path, extension } = file

        if (fm.fileCallback) {
            fm.url({ disk: disk(), path }).then((response) => {
                if (response.data.result.status === 'success') {
                    fm.fileCallback(response.data.url)
                }
            })
            noteOpened(file, false)

            return
        }

        const customEditor = getDefaultEditor(file)
        if (customEditor && (!isFileTooLarge(file) || loadsOwnContent(customEditor.editor))) {
            modal.openPluginEditor({
                pluginId: customEditor.pluginId,
                editor: customEditor.editor,
                file,
            })
            noteOpened(file)

            return
        }

        if (!extension) return

        const ext = extension.toLowerCase()
        if (settings.imageExtensions.includes(ext)) {
            openSelectionModal(file, 'PreviewModal')
        } else if (Object.keys(settings.textExtensions).includes(ext)) {
            openSelectionModal(file, 'TextEditModal')
        } else if (settings.audioExtensions.includes(ext)) {
            openSelectionModal(file, 'AudioPlayerModal')
        } else if (settings.videoExtensions.includes(ext)) {
            openSelectionModal(file, 'VideoPlayerModal')
        } else if (ext === 'pdf') {
            fm.openPDF({ disk: disk(), path })
            noteOpened(file)
        }
    }

    return { openFile }
}
