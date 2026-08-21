<template>
    <div
        ref="contextMenu"
        v-if="menuVisible"
        v-bind:style="menuStyle"
        v-on:blur="closeMenu"
        class="fm-context-menu"
        tabindex="-1"
    >
        <template v-for="block in menuBlocks" v-bind:key="`g-${block.group}`">
            <ul v-if="block.items.length || block.editors.length" class="list-unstyled">
                <li v-for="item in block.items" v-on:click="menuAction(item.name)" v-bind:key="`i-${item.name}`">
                    <span class="fm-context-menu-icon"><GIcon :name="item.icon" :class="item.iconClass" /></span>
                    {{ lang.contextMenu[item.name] }}
                </li>
                <li
                    v-for="(editorItem, idx) in block.editors"
                    :key="`pe-${idx}`"
                    :class="{ disabled: editorItem.disabled }"
                    :title="editorItem.disabled ? lang.contextMenu.fileTooLarge : ''"
                    @click="!editorItem.disabled && openPluginEditor(editorItem)"
                >
                    <span class="fm-context-menu-icon"><GIcon :name="editorItem.editor.icon || 'edit'" /></span>
                    {{ getEditorMenuLabel(editorItem) }}
                </li>
            </ul>
        </template>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { GIcon } from '@gameap/ui'
import EventBus from '../../emitter.js'
import { isExtractable } from '../../archive.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../stores/useSettingsStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useTranslate } from '../../composables/useTranslate.js'
import { useFileEditors, isFileTooLarge, loadsOwnContent } from '../../composables/useFileEditors.js'
import { usePluginsStore } from '../../../store/plugins'

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

function downloadDirRule() {
    return !multiSelect.value && firstItemType.value === 'dir'
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

function chmodRule() {
    return selectedItems.value.length > 0
}

function pasteRule() {
    return !!fm.clipboard.type
}

function zipRule() {
    return selectedItems.value.length > 0
}

function unzipRule() {
    return (
        !multiSelect.value &&
        firstItemType.value === 'file' &&
        isExtractable(selectedItems.value[0]?.basename)
    )
}

function hashRule() {
    return selectedItems.value.length === 1 && selectedItems.value[0].type === 'file'
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
    downloadDir: downloadDirRule,
    copy: copyRule,
    cut: cutRule,
    rename: renameRule,
    chmod: chmodRule,
    paste: pasteRule,
    zip: zipRule,
    unzip: unzipRule,
    hash: hashRule,
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
    fm.download({
        disk: selectedDisk.value,
        path: selectedItems.value[0].path,
        filename: selectedItems.value[0].basename,
    })
}

function downloadDirAction() {
    const item = selectedItems.value[0]
    const archiveName = `${item.basename || item.name || 'archive'}.zip`
    fm.downloadDirectory({
        disk: selectedDisk.value,
        path: item.path,
        filename: archiveName,
    }).catch(() => {
        /* errors are surfaced via the messages store */
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

function chmodAction() {
    modal.setModalState({ modalName: 'ChmodModal', show: true })
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

function hashAction() {
    modal.setModalState({ modalName: 'HashModal', show: true })
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
    downloadDir: downloadDirAction,
    copy: copyAction,
    cut: cutAction,
    rename: renameAction,
    chmod: chmodAction,
    paste: pasteAction,
    zip: zipAction,
    unzip: unzipAction,
    hash: hashAction,
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
        // The size cap is about handing the editor the file's content; an
        // editor that loads what it needs itself is offered whatever the size.
        disabled: fileTooLarge && !loadsOwnContent(item.editor)
    }))
})

// Where a plugin item goes when it names no block, and when it names one this
// panel does not know: the block plugin items had to themselves before there
// was a choice of any. An item in an unexpected block is still an item, a
// dropped one is a bug.
const PLUGIN_GROUP = 'top'

const menuBlocks = computed(() => {
    const editors = new Map(
        [PLUGIN_GROUP, ...settings.contextMenu.map((block) => block.group)].map((group) => [group, []])
    )
    for (const item of pluginEditorItems.value) {
        const named = item.editor.menuGroup
        editors.get(editors.has(named) ? named : PLUGIN_GROUP).push(item)
    }
    return [{ group: PLUGIN_GROUP, items: [] }, ...settings.contextMenu].map(({ group, items }) => ({
        group,
        // Filtered here rather than in the template, so a block left with
        // nothing to show goes away together with the divider it would draw.
        items: items.filter((item) => showMenuItem(item.name)),
        editors: editors.get(group),
    }))
})

function getEditorMenuLabel(editorItem) {
    // An editor that is not "Edit with X" — a viewer, a comparison — names its
    // own item, and that name goes through the plugin's translations.
    if (editorItem.editor.menuLabel) {
        return pluginsStore.resolvePluginText(editorItem.pluginId, editorItem.editor.menuLabel)
    }
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
    @apply bg-white dark:bg-stone-900 rounded border shadow-lg;

    position: absolute;
    z-index: 9997;
    overflow: hidden;

    &:focus {
        outline: none;
    }

    .list-unstyled {
        @apply border-b;
        margin-bottom: 0;

        &:last-child {
            border-bottom: none;
        }
    }

    ul > li {
        padding: 0.4rem 1rem;
    }

    ul > li:not(.disabled) {
        cursor: pointer;

        &:hover {
          @apply bg-surface-hover;
        }
    }

    ul > li.disabled {
        @apply text-faint;
        cursor: not-allowed;
    }

    // Glyph widths vary between icons; a fixed-width slot keeps every label
    // starting at the same offset.
    .fm-context-menu-icon {
        display: inline-block;
        width: 1.25em;
        margin-right: 1.5rem;
        text-align: center;
    }
}
</style>
