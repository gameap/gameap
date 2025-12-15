<template>
    <div class="fm-table">
        <table class="table table-sm w-full">
            <thead>
                <tr>
                    <th class="w-65" v-on:click="handleSortBy('name')">
                        {{ lang.manager.table.name }}
                        <template v-if="sortSettings.field === 'name'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-10" v-on:click="handleSortBy('size')">
                        {{ lang.manager.table.size }}
                        <template v-if="sortSettings.field === 'size'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-10" v-on:click="handleSortBy('type')">
                        {{ lang.manager.table.type }}
                        <template v-if="sortSettings.field === 'type'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-auto" v-on:click="handleSortBy('date')">
                        {{ lang.manager.table.date }}
                        <template v-if="sortSettings.field === 'date'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr v-if="!isRootPath">
                    <td colspan="4" class="fm-content-item" v-on:click="levelUp">
                        <i class="fa-solid fa-arrow-turn-up"></i>
                    </td>
                </tr>
                <tr
                    v-for="(directory, index) in directories"
                    v-bind:key="`d-${index}`"
                    v-bind:class="{ 'table-info': checkSelect('directories', directory.path) }"
                    v-on:click="selectItem('directories', directory.path, $event)"
                    v-on:contextmenu.prevent="contextMenu(directory, $event)"
                >
                    <td
                        class="fm-content-item unselectable"
                        v-bind:class="acl && directory.acl === 0 ? 'text-hidden' : ''"
                        v-on:dblclick="handleSelectDirectory(directory.path)"
                    >
                        <i class="fa-regular fa-folder"></i> {{ directory.basename }}
                    </td>
                    <td />
                    <td>{{ lang.manager.table.folder }}</td>
                    <td>
                        {{ timestampToDate(directory.timestamp) }}
                    </td>
                </tr>
                <tr
                    v-for="(file, index) in files"
                    v-bind:key="`f-${index}`"
                    v-bind:class="{ 'table-info': checkSelect('files', file.path) }"
                    v-on:click="selectItem('files', file.path, $event)"
                    v-on:dblclick="selectAction(file)"
                    v-on:contextmenu.prevent="contextMenu(file, $event)"
                >
                    <td class="fm-content-item unselectable" v-bind:class="acl && file.acl === 0 ? 'text-hidden' : ''">
                        <i v-bind:class="extensionToIcon(file.extension)" />
                        {{ file.basename }}
                    </td>
                    <td>{{ bytesToHuman(file.size) }}</td>
                    <td>
                        {{ file.extension }}
                    </td>
                    <td>
                        {{ timestampToDate(file.timestamp) }}
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import EventBus from '../../emitter.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../stores/useSettingsStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useManager } from '../../composables/useManager.js'
import { useTranslate } from '../../composables/useTranslate.js'
import { useHelper } from '../../composables/useHelper.js'
import { useFileEditors } from '../../composables/useFileEditors.js'

const props = defineProps({
    manager: { type: String, required: true },
})

const fm = useFileManagerStore()
const settings = useSettingsStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { bytesToHuman, timestampToDate, extensionToIcon } = useHelper()
const { getDefaultEditor, isFileTooLarge } = useFileEditors()

const {
    selectedDisk,
    selectedDirectory,
    files,
    directories,
    selected,
    sort,
    selectDirectory,
    sortBy,
} = useManager(props.manager)

const sortSettings = computed(() => sort.value)
const acl = computed(() => settings.acl)
const isRootPath = computed(() => selectedDirectory.value === null)

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

function handleSortBy(field) {
    sortBy(field, null)
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
.fm-table {
    thead th {
        @apply text-left bg-white dark:bg-stone-800;

        position: sticky;
        top: 0;
        z-index: 10;
        cursor: pointer;
        border-top: none;

        &:hover {
          @apply bg-stone-100 dark:bg-[#262322];
        }

        & > i {
            padding-left: 0.5rem;
        }
    }

    td {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    tr:nth-child(odd) {
        @apply bg-stone-50 dark:bg-stone-800;
    }

    tr:nth-child(even) {
        @apply bg-white dark:bg-stone-900;
    }

    tr:hover {
        @apply bg-stone-100 dark:bg-[#3d3836];
    }

    .w-10 {
        width: 10%;
    }

    .w-65 {
        width: 65%;
    }

    .fm-content-item {
        @apply px-2 py-3;
        cursor: pointer;
    }

    .text-hidden {
        color: #cdcdcd;
    }
}
</style>
