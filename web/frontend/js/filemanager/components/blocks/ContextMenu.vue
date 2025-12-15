<template>
    <div
        ref="contextMenu"
        v-if="menuVisible"
        v-bind:style="menuStyle"
        v-on:blur="closeMenu"
        class="fm-context-menu"
        tabindex="-1"
    >
        <ul v-if="pluginEditorItems.length > 0" class="list-unstyled">
          <li
              v-for="(editorItem, idx) in pluginEditorItems"
              :key="`pe-${idx}`"
              :class="{ disabled: editorItem.disabled }"
              :title="editorItem.disabled ? lang.contextMenu.fileTooLarge : ''"
              @click="!editorItem.disabled && openPluginEditor(editorItem)"
          >
            <i :class="editorItem.editor.icon || 'fa-solid fa-pen-ruler'" />
            {{ getEditorMenuLabel(editorItem) }}
          </li>
        </ul>
        <ul v-for="(group, index) in menu" v-bind:key="`g-${index}`" class="list-unstyled">
            <template v-for="(item, idx) in group">
                <li v-if="showMenuItem(item.name)" v-on:click="menuAction(item.name)" v-bind:key="`i-${idx}`">
                    <i v-bind:class="item.icon" />
                    {{ lang.contextMenu[item.name] }}
                </li>
            </template>
        </ul>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import EventBus from '../../emitter.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../stores/useSettingsStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useTranslate } from '../../composables/useTranslate.js'
import { useFileEditors, isFileTooLarge } from '../../composables/useFileEditors.js'
import { usePluginsStore } from '../../../store/plugins'
import HTTP from '../../http/get.js'

const fm = useFileManagerStore()
const pluginsStore = usePluginsStore()
const settings = useSettingsStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { getMatchingEditors } = useFileEditors()

const contextMenu = ref(null)
const menuVisible = ref(false)
const menuStyle = ref({
    top: 0,
    left: 0,
})

const menu = computed(() => settings.contextMenu)
const selectedDisk = computed(() => fm.selectedDisk)
const selectedItems = computed(() => fm.selectedItems)
const selectedDiskDriver = computed(() => fm.disks[selectedDisk.value]?.driver)
const multiSelect = computed(() => selectedItems.value.length > 1)
const firstItemType = computed(() => selectedItems.value[0]?.type)

function canView(extension) {
    if (!extension) return false
    return settings.imageExtensions.includes(extension.toLowerCase())
}

function canEdit(extension) {
    if (!extension) return false
    return Object.keys(settings.textExtensions).includes(extension.toLowerCase())
}

function canAudioPlay(extension) {
    if (!extension) return false
    return settings.audioExtensions.includes(extension.toLowerCase())
}

function canVideoPlay(extension) {
    if (!extension) return false
    return settings.videoExtensions.includes(extension.toLowerCase())
}

function isZip(extension) {
    if (!extension) return false
    return extension.toLowerCase() === 'zip'
}

// Rules
function openRule() {
    return !multiSelect.value && firstItemType.value === 'dir'
}

function audioPlayRule() {
    return (
        selectedItems.value.every((elem) => elem.type === 'file') &&
        selectedItems.value.every((elem) => canAudioPlay(elem.extension))
    )
}

function videoPlayRule() {
    return !multiSelect.value && canVideoPlay(selectedItems.value[0]?.extension)
}

function viewRule() {
    return !multiSelect.value && firstItemType.value === 'file' && canView(selectedItems.value[0]?.extension)
}

function editRule() {
    return !multiSelect.value && firstItemType.value === 'file' && canEdit(selectedItems.value[0]?.extension)
}

function selectRule() {
    return !multiSelect.value && firstItemType.value === 'file' && fm.fileCallback
}

function downloadRule() {
    return !multiSelect.value && firstItemType.value === 'file'
}

function copyRule() {
    return true
}

function cutRule() {
    return true
}

function renameRule() {
    return !multiSelect.value
}

function pasteRule() {
    return !!fm.clipboard.type
}

function zipRule() {
    return selectedDiskDriver.value === 'local'
}

function unzipRule() {
    return (
        selectedDiskDriver.value === 'local' &&
        !multiSelect.value &&
        firstItemType.value === 'file' &&
        isZip(selectedItems.value[0]?.extension)
    )
}

function deleteRule() {
    return true
}

function propertiesRule() {
    return !multiSelect.value
}

const rules = {
    open: openRule,
    audioPlay: audioPlayRule,
    videoPlay: videoPlayRule,
    view: viewRule,
    edit: editRule,
    select: selectRule,
    download: downloadRule,
    copy: copyRule,
    cut: cutRule,
    rename: renameRule,
    paste: pasteRule,
    zip: zipRule,
    unzip: unzipRule,
    delete: deleteRule,
    properties: propertiesRule,
}

// Actions
function openAction() {
    fm.selectDirectory(fm.activeManager, {
        path: selectedItems.value[0].path,
        history: true,
    })
}

function audioPlayAction() {
    modal.setModalState({ modalName: 'AudioPlayerModal', show: true })
}

function videoPlayAction() {
    modal.setModalState({ modalName: 'VideoPlayerModal', show: true })
}

function viewAction() {
    modal.setModalState({ modalName: 'PreviewModal', show: true })
}

function editAction() {
    modal.setModalState({ modalName: 'TextEditModal', show: true })
}

function selectAction() {
    fm.url({ disk: selectedDisk.value, path: selectedItems.value[0].path }).then((response) => {
        if (response.data.result.status === 'success') {
            fm.fileCallback(response.data.url)
        }
    })
}

function downloadAction() {
    const tempLink = document.createElement('a')
    tempLink.style.display = 'none'
    tempLink.setAttribute('download', selectedItems.value[0].basename)

    HTTP.download(selectedDisk.value, selectedItems.value[0].path).then((response) => {
        tempLink.href = window.URL.createObjectURL(new Blob([response.data]))
        document.body.appendChild(tempLink)
        tempLink.click()
        document.body.removeChild(tempLink)
    })
}

function copyAction() {
    fm.toClipboard('copy')
}

function cutAction() {
    fm.toClipboard('cut')
}

function renameAction() {
    modal.setModalState({ modalName: 'RenameModal', show: true })
}

function pasteAction() {
    fm.paste()
}

function zipAction() {
    modal.setModalState({ modalName: 'ZipModal', show: true })
}

function unzipAction() {
    modal.setModalState({ modalName: 'UnzipModal', show: true })
}

function deleteAction() {
    modal.setModalState({ modalName: 'DeleteModal', show: true })
}

function propertiesAction() {
    modal.setModalState({ modalName: 'PropertiesModal', show: true })
}

const actions = {
    open: openAction,
    audioPlay: audioPlayAction,
    videoPlay: videoPlayAction,
    view: viewAction,
    edit: editAction,
    select: selectAction,
    download: downloadAction,
    copy: copyAction,
    cut: cutAction,
    rename: renameAction,
    paste: pasteAction,
    zip: zipAction,
    unzip: unzipAction,
    delete: deleteAction,
    properties: propertiesAction,
}

function showMenu(event) {
    if (selectedItems.value.length) {
        menuVisible.value = true

        nextTick(() => {
            contextMenu.value?.focus()
            setMenu(event.pageY, event.pageX)
        })
    }
}

function setMenu(top, left) {
    const el = contextMenu.value?.parentNode
    if (!el) return

    const elSize = el.getBoundingClientRect()
    const elY = window.scrollY + elSize.top
    const elX = window.scrollX + elSize.left

    let menuY = top - elY
    let menuX = left - elX

    const maxY = elY + (el.offsetHeight - contextMenu.value.offsetHeight - 25)
    const maxX = elX + (el.offsetWidth - contextMenu.value.offsetWidth - 25)

    if (top > maxY) menuY = maxY - elY
    if (left > maxX) menuX = maxX - elX

    menuStyle.value.top = `${menuY}px`
    menuStyle.value.left = `${menuX}px`
}

function closeMenu() {
    menuVisible.value = false
}

function showMenuItem(name) {
    if (rules[name]) {
        return rules[name]()
    }
    return false
}

function menuAction(name) {
    if (actions[name]) {
        actions[name]()
    }
    closeMenu()
}

const pluginEditorItems = computed(() => {
    if (multiSelect.value || firstItemType.value !== 'file') {
        return []
    }
    const file = selectedItems.value[0]
    if (!file) return []

    const fileTooLarge = isFileTooLarge(file)
    return getMatchingEditors(file).map(item => ({
        ...item,
        disabled: fileTooLarge
    }))
})

function getEditorMenuLabel(editorItem) {
    const baseName = pluginsStore.resolvePluginText(editorItem.pluginId, editorItem.editor.name)
    if (editorItem.isDefault) {
        return `Edit with ${baseName} (default)`
    }
    return `Edit with ${baseName}`
}

function openPluginEditor(editorItem) {
    modal.openPluginEditor({
        pluginId: editorItem.pluginId,
        editor: editorItem.editor,
        file: selectedItems.value[0]
    })
    closeMenu()
}

onMounted(() => {
    EventBus.on('contextMenu', (event) => showMenu(event))
})
</script>

<style lang="scss">
.fm-context-menu {
    @apply bg-white dark:bg-stone-900;

    position: absolute;
    z-index: 9997;
    border-radius: 5px;

    &:focus {
        outline: none;
    }

    .list-unstyled {
        margin-bottom: 0;
        border-bottom: 1px solid rgba(0, 0, 0, 0.125);
    }

    ul > li {
        padding: 0.4rem 1rem;
    }

    ul > li:not(.disabled) {
        cursor: pointer;

        &:hover {
          @apply bg-stone-100 dark:bg-[#262322];
        }

        i {
            padding-right: 1.5rem;
        }
    }

    ul > li.disabled {
        @apply text-stone-400 dark:text-stone-600;
        cursor: not-allowed;

        i {
            padding-right: 1.5rem;
        }
    }
}
</style>
