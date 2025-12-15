import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useModalStore = defineStore('fm-modal', () => {
    const showModal = ref(false)
    const modalName = ref(null)
    const pluginEditorState = ref(null)

    function setModalState({ show, modalName: name }) {
        showModal.value = show
        modalName.value = name
    }

    function openPluginEditor({ pluginId, editor, file }) {
        pluginEditorState.value = { pluginId, editor, file }
        showModal.value = true
        modalName.value = 'PluginEditorModal'
    }

    function clearModal() {
        showModal.value = false
        modalName.value = null
        pluginEditorState.value = null
    }

    return {
        showModal,
        modalName,
        pluginEditorState,
        setModalState,
        openPluginEditor,
        clearModal,
    }
})
