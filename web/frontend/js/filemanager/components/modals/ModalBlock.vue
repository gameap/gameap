<template>
    <GModal
        v-model:show="showModal"
        :title="currentModalConfig?.title"
        :style="modalStyle"
        :content-style="currentModalConfig?.fitViewport ? FIT_VIEWPORT_CONTENT_STYLE : undefined"
        transform-origin="center"
    >
        <component :is="modalComponents[modalName]" ref="modalRef" />
        <template #footer v-if="footerButtons.length">
            <div class="flex justify-end gap-2">
                <GButton
                    v-for="(btn, i) in footerButtons"
                    :key="i"
                    :color="btn.color"
                    :disabled="btn.disabled"
                    @click="btn.action"
                >
                    <GIcon v-if="btn.icon" :name="btn.icon" class="mr-1" />
                    {{ btn.label }}
                </GButton>
            </div>
        </template>
    </GModal>
</template>

<script setup>
import { ref, computed } from 'vue'
import { GIcon, GModal } from '@gameap/ui'
import { useModalStore } from '@/filemanager/stores'
import { useTranslate } from '@/filemanager/composables'
import { usePluginsStore } from '@/store/plugins'
import GButton from '@/components/GButton.vue'
import NewFileModal from './views/NewFileModal.vue'
import NewFolderModal from './views/NewFolderModal.vue'
import UploadModal from './views/UploadModal.vue'
import DeleteModal from './views/DeleteModal.vue'
import ClipboardModal from './views/ClipboardModal.vue'
import StatusModal from './views/StatusModal.vue'
import RenameModal from './views/RenameModal.vue'
import ChmodModal from './views/ChmodModal.vue'
import PropertiesModal from './views/PropertiesModal.vue'
import PreviewModal from './views/PreviewModal.vue'
import TextEditModal from './views/TextEditModal.vue'
import AudioPlayerModal from './views/AudioPlayerModal.vue'
import VideoPlayerModal from './views/VideoPlayerModal.vue'
import AboutModal from './views/AboutModal.vue'
import PluginEditorModal from './views/PluginEditorModal.vue'
import HashModal from './views/HashModal.vue'
import ZipModal from './views/ZipModal.vue'
import UnzipModal from './views/UnzipModal.vue'

// Modals listing an unbounded number of items stay within the viewport, so
// the footer buttons are always on screen: the card is capped and its body
// becomes a flex column, where the list (min-h-0 overflow-auto) takes the
// space left and scrolls. The body itself scrolls only when even the fixed
// parts do not fit.
const FIT_VIEWPORT_CONTENT_STYLE = {
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
    overflowY: 'auto',
}

const modal = useModalStore()
const pluginsStore = usePluginsStore()
const { lang } = useTranslate()
const modalRef = ref(null)

const modalComponents = {
    NewFileModal,
    NewFolderModal,
    UploadModal,
    DeleteModal,
    ClipboardModal,
    StatusModal,
    RenameModal,
    ChmodModal,
    PropertiesModal,
    PreviewModal,
    TextEditModal,
    AudioPlayerModal,
    VideoPlayerModal,
    AboutModal,
    PluginEditorModal,
    HashModal,
    ZipModal,
    UnzipModal,
}

const showModal = computed({
    get: () => modal.showModal,
    set: (val) => {
        if (!val) modal.clearModal()
    },
})

const modalName = computed(() => modal.modalName)

const modalConfig = computed(() => ({
    NewFileModal: { title: lang.value.modal.newFile.title, width: '600px' },
    NewFolderModal: { title: lang.value.modal.newFolder.title, width: '600px' },
    UploadModal: { title: lang.value.modal.upload.title, width: '760px', fitViewport: true },
    DeleteModal: { title: lang.value.modal.delete.title, width: '600px', fitViewport: true },
    ClipboardModal: { title: lang.value.clipboard.title, width: '600px' },
    StatusModal: { title: lang.value.modal.status.title, width: '600px' },
    RenameModal: { title: lang.value.modal.rename.title, width: '600px' },
    ChmodModal: { title: lang.value.modal.chmod.title, width: '600px' },
    PropertiesModal: { title: lang.value.modal.properties.title, width: '600px' },
    PreviewModal: { title: lang.value.modal.preview.title, width: '1000px' },
    TextEditModal: { title: lang.value.modal.editor.title, width: '1000px' },
    AudioPlayerModal: { title: lang.value.modal.audioPlayer.title, width: '600px' },
    VideoPlayerModal: { title: lang.value.modal.videoPlayer.title, width: '800px' },
    AboutModal: { title: lang.value.modal.about.title, width: '600px' },
    // The width comes off the editor definition rather than from this map:
    // a plugin that shows two panes side by side needs more room than one that
    // shows a form, and the store is filled before the modal renders, so there
    // is no resize on the first frame.
    PluginEditorModal: { title: pluginsStore.resolvePluginText(modal.pluginEditorState?.pluginId, modal.pluginEditorState?.editor?.name) || lang.value.modal.editor.title, width: modal.pluginEditorState?.editor?.width || '1000px' },
    HashModal: { title: lang.value.modal.hash.title, width: '700px' },
    ZipModal: { title: lang.value.modal.zip.title, width: '600px' },
    UnzipModal: { title: lang.value.modal.unzip.title, width: '600px' },
}))

const currentModalConfig = computed(() => modalConfig.value[modalName.value])

const modalStyle = computed(() => {
    const style = { width: currentModalConfig.value?.width || '600px' }
    if (currentModalConfig.value?.fitViewport) style.maxHeight = 'calc(100dvh - 4rem)'

    return style
})

const footerButtons = computed(() => modalRef.value?.footerButtons || [])
</script>
