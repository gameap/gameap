<template>
    <n-modal
        v-model:show="showModal"
        class="custom-card"
        preset="card"
        :title="currentModalConfig?.title"
        :bordered="false"
        :style="{ width: currentModalConfig?.width || '600px' }"
        :segmented="{ content: 'soft', footer: 'soft' }"
        transform-origin="center"
    >
        <component :is="modalComponents[modalName]" ref="modalRef" />
        <template #footer v-if="footerButtons.length">
            <GButton
                v-for="(btn, i) in footerButtons"
                :key="i"
                :color="btn.color"
                :disabled="btn.disabled"
                :class="{ 'mr-1': i < footerButtons.length - 1 }"
                @click="btn.action"
            >
                <i v-if="btn.icon" :class="[btn.icon, 'mr-1']" />
                {{ btn.label }}
            </GButton>
        </template>
    </n-modal>
</template>

<script setup>
import { ref, computed } from 'vue'
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
import PropertiesModal from './views/PropertiesModal.vue'
import PreviewModal from './views/PreviewModal.vue'
import TextEditModal from './views/TextEditModal.vue'
import AudioPlayerModal from './views/AudioPlayerModal.vue'
import VideoPlayerModal from './views/VideoPlayerModal.vue'
import ZipModal from './views/ZipModal.vue'
import UnzipModal from './views/UnzipModal.vue'
import AboutModal from './views/AboutModal.vue'
import PluginEditorModal from './views/PluginEditorModal.vue'

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
    PropertiesModal,
    PreviewModal,
    TextEditModal,
    AudioPlayerModal,
    VideoPlayerModal,
    ZipModal,
    UnzipModal,
    AboutModal,
    PluginEditorModal,
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
    UploadModal: { title: lang.value.modal.upload.title, width: '600px' },
    DeleteModal: { title: lang.value.modal.delete.title, width: '600px' },
    ClipboardModal: { title: lang.value.clipboard.title, width: '600px' },
    StatusModal: { title: lang.value.modal.status.title, width: '600px' },
    RenameModal: { title: lang.value.modal.rename.title, width: '600px' },
    PropertiesModal: { title: lang.value.modal.properties.title, width: '600px' },
    PreviewModal: { title: lang.value.modal.preview.title, width: '1000px' },
    TextEditModal: { title: lang.value.modal.editor.title, width: '1000px' },
    AudioPlayerModal: { title: lang.value.modal.audioPlayer.title, width: '600px' },
    VideoPlayerModal: { title: lang.value.modal.videoPlayer.title, width: '800px' },
    ZipModal: { title: lang.value.modal.zip.title, width: '600px' },
    UnzipModal: { title: lang.value.modal.unzip.title, width: '600px' },
    AboutModal: { title: lang.value.modal.about.title, width: '600px' },
    PluginEditorModal: { title: pluginsStore.resolvePluginText(modal.pluginEditorState?.pluginId, modal.pluginEditorState?.editor?.name) || lang.value.modal.editor.title, width: '1000px' },
}))

const currentModalConfig = computed(() => modalConfig.value[modalName.value])

const footerButtons = computed(() => modalRef.value?.footerButtons || [])
</script>
