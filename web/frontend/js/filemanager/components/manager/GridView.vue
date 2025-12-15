<template>
    <div class="fm-grid">
        <div class="grid grid-cols-2 md:grid-cols-8 md:gap-2 sm:grid-cols-4 sm:gap-1">
            <div v-if="!isRootPath" v-on:click="levelUp" class="fm-grid-item text-center hover:bg-stone-50">
                <div class="fm-item-icon">
                    <i class="fa-solid fa-arrow-turn-up pb-2"></i>
                </div>
                <div class="fm-item-info"><strong>..</strong></div>
            </div>

            <div
                class="fm-grid-item text-center unselectable"
                v-for="(directory, index) in directories"
                v-bind:key="`d-${index}`"
                v-bind:title="directory.basename"
                v-bind:class="{ active: checkSelect('directories', directory.path) }"
                v-on:click="selectItem('directories', directory.path, $event)"
                v-on:dblclick.stop="handleSelectDirectory(directory.path)"
                v-on:contextmenu.prevent="contextMenu(directory, $event)"
            >
                <div class="fm-item-icon">
                    <i class="fa-regular pb-2" v-bind:class="acl && directory.acl === 0 ? 'fa-lock' : 'fa-folder'" />
                </div>
                <div class="fm-item-info">{{ directory.basename }}</div>
            </div>

            <div
                class="fm-grid-item text-center unselectable"
                v-for="(file, index) in files"
                v-bind:key="`f-${index}`"
                v-bind:title="file.basename"
                v-bind:class="{ active: checkSelect('files', file.path) }"
                v-on:click="selectItem('files', file.path, $event)"
                v-on:dblclick="selectAction(file)"
                v-on:contextmenu.prevent="contextMenu(file, $event)"
            >
                <div class="fm-item-icon">
                    <i v-if="acl && file.acl === 0" class="fa-solid fa-file-circle-xmark pb-2" />
                    <thumbnail v-else-if="thisImage(file.extension)" v-bind:disk="disk" v-bind:file="file"></thumbnail>
                    <i v-else class="pb-2" v-bind:class="extensionToIcon(file.extension)" />
                </div>
                <div class="fm-item-info">
                    {{ `${file.filename}.${file.extension}` }}
                    <br />
                    {{ bytesToHuman(file.size) }}
                </div>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import EventBus from '../../emitter.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../stores/useSettingsStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useManager } from '../../composables/useManager.js'
import { useHelper } from '../../composables/useHelper.js'
import { useFileEditors } from '../../composables/useFileEditors.js'
import Thumbnail from './Thumbnail.vue'

const props = defineProps({
    manager: { type: String, required: true },
})

const fm = useFileManagerStore()
const settings = useSettingsStore()
const modal = useModalStore()
const { bytesToHuman, extensionToIcon } = useHelper()
const { getDefaultEditor, isFileTooLarge } = useFileEditors()

const {
    selectedDisk,
    selectedDirectory,
    files,
    directories,
    selected,
    selectDirectory,
} = useManager(props.manager)

const disk = ref('')

const acl = computed(() => settings.acl)
const isRootPath = computed(() => selectedDirectory.value === null)
const imageExtensions = computed(() => settings.imageExtensions)

onMounted(() => {
    disk.value = selectedDisk.value
})

watch(selectedDisk, (newVal) => {
    if (disk.value !== newVal) {
        disk.value = newVal
    }
})

function thisImage(extension) {
    if (!extension) return false
    return imageExtensions.value.includes(extension.toLowerCase())
}

function levelUp() {
    if (selectedDirectory.value) {
        const pathUp = selectedDirectory.value.split('/').slice(0, -1).join('/')
        selectDirectory(pathUp || null, true)
    }
}

function checkSelect(type, path) {
    return selected.value[type].includes(path)
}

function selectItem(type, path, event) {
    const alreadySelected = selected.value[type].includes(path)

    if (event.ctrlKey || event.metaKey) {
        if (!alreadySelected) {
            fm.addToSelection(props.manager, { type, path })
        } else {
            fm.removeFromSelection(props.manager, { type, path })
        }
    }

    if (!event.ctrlKey && !alreadySelected && !event.metaKey) {
        fm.changeSelected(props.manager, { type, path })
    }
}

function contextMenu(item, event) {
    const type = item.type === 'dir' ? 'directories' : 'files'
    const alreadySelected = selected.value[type].includes(item.path)

    if (!alreadySelected) {
        fm.changeSelected(props.manager, { type, path: item.path })
    }

    EventBus.emit('contextMenu', event)
}

function handleSelectDirectory(path) {
    selectDirectory(path, true)
}

function selectAction(file) {
    const { path, extension } = file

    if (fm.fileCallback) {
        fm.url({ disk: selectedDisk.value, path }).then((response) => {
            if (response.data.result.status === 'success') {
                fm.fileCallback(response.data.url)
            }
        })
        return
    }

    const customEditor = getDefaultEditor(file)
    if (customEditor && !isFileTooLarge(file)) {
        modal.openPluginEditor({
            pluginId: customEditor.pluginId,
            editor: customEditor.editor,
            file: file
        })
        return
    }

    if (!extension) return

    if (settings.imageExtensions.includes(extension.toLowerCase())) {
        modal.setModalState({ modalName: 'PreviewModal', show: true })
    } else if (Object.keys(settings.textExtensions).includes(extension.toLowerCase())) {
        modal.setModalState({ modalName: 'TextEditModal', show: true })
    } else if (settings.audioExtensions.includes(extension.toLowerCase())) {
        modal.setModalState({ modalName: 'AudioPlayerModal', show: true })
    } else if (settings.videoExtensions.includes(extension.toLowerCase())) {
        modal.setModalState({ modalName: 'VideoPlayerModal', show: true })
    } else if (extension.toLowerCase() === 'pdf') {
        fm.openPDF({ disk: selectedDisk.value, path })
    }
}
</script>

<style lang="scss">
.fm-grid {
    padding-top: 1rem;

    .fm-grid-item {
        //position: relative;
        width: 125px;
        padding: 0.4rem;
        margin-bottom: 1rem;
        margin-right: 1rem;
        border-radius: 5px;

        &.active {
            @apply bg-stone-200;
        }

        &:not(.active):hover {
            @apply bg-stone-100;
        }

        .fm-item-icon {
            font-size: 4rem;
            cursor: pointer;
        }

        .fm-item-icon > i,
        .fm-item-icon > figure > i {
            //color: #6c757d;
        }

        .fm-item-info {
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
    }
}
</style>
