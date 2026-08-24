<template>
    <div
        ref="tableArea"
        class="fm-table-area"
        tabindex="0"
        v-on:click.self="clearOnEmpty"
        v-on:keydown="onKeyDown"
    >
        <table class="fm-table">
            <thead>
                <tr>
                    <th
                        class="fm-th fm-th--name"
                        v-bind:class="{ 'fm-th--active': sortSettings.field === 'name' }"
                        v-on:click="handleSortBy('name')"
                    >
                        <span class="fm-th-label">{{ lang.manager.table.name }}</span>
                        <GIcon
                            class="fm-th-sort"
                            v-bind:class="{ 'fm-th-sort--inactive': sortSettings.field !== 'name' }"
                            v-bind:name="sortIcon('name')"
                        />
                    </th>
                    <th
                        class="fm-th fm-th--size"
                        v-bind:class="{ 'fm-th--active': sortSettings.field === 'size' }"
                        v-on:click="handleSortBy('size')"
                    >
                        <span class="fm-th-label">{{ lang.manager.table.size }}</span>
                        <GIcon
                            class="fm-th-sort"
                            v-bind:class="{ 'fm-th-sort--inactive': sortSettings.field !== 'size' }"
                            v-bind:name="sortIcon('size')"
                        />
                    </th>
                    <th
                        class="fm-th fm-th--type"
                        v-bind:class="{ 'fm-th--active': sortSettings.field === 'type' }"
                        v-on:click="handleSortBy('type')"
                    >
                        <span class="fm-th-label">{{ lang.manager.table.type }}</span>
                        <GIcon
                            class="fm-th-sort"
                            v-bind:class="{ 'fm-th-sort--inactive': sortSettings.field !== 'type' }"
                            v-bind:name="sortIcon('type')"
                        />
                    </th>
                    <th
                        class="fm-th fm-th--date"
                        v-bind:class="{ 'fm-th--active': sortSettings.field === 'date' }"
                        v-on:click="handleSortBy('date')"
                    >
                        <span class="fm-th-label">{{ lang.manager.table.date }}</span>
                        <GIcon
                            class="fm-th-sort"
                            v-bind:class="{ 'fm-th-sort--inactive': sortSettings.field !== 'date' }"
                            v-bind:name="sortIcon('date')"
                        />
                    </th>
                    <th class="fm-th fm-th--perm">
                        {{ lang.manager.table.permissions }}
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr v-if="!isRootPath" class="fm-row fm-row--up" v-on:click="levelUp">
                    <td colspan="5" class="fm-content-item">
                        <GIcon name="arrow-turn-up" />
                        <span class="ml-2 text-muted">..</span>
                    </td>
                </tr>
                <tr
                    v-for="(directory, index) in directories"
                    v-bind:key="`d-${directory.path}`"
                    v-bind:ref="(el) => registerRow(el, directoryRowIndex(index))"
                    class="fm-row fm-row--directory"
                    v-bind:class="{
                        'fm-row--zebra': directoryRowIndex(index) % 2 === 1,
                        'fm-row--selected': checkSelect('directories', directory.path),
                        'fm-row--focused': focusedIndex === directoryRowIndex(index),
                        'fm-row--search-current': currentMatchIndex === directoryRowIndex(index),
                        'fm-row--locked': acl && directory.acl === 0,
                    }"
                    tabindex="-1"
                    v-on:click="selectItem('directories', directory.path, $event, directoryRowIndex(index))"
                    v-on:dblclick="handleSelectDirectory(directory.path)"
                    v-on:contextmenu.prevent="contextMenu(directory, $event)"
                >
                    <td class="fm-content-item unselectable">
                        <span class="fm-row-icon">
                            <GIcon
                                v-if="checkSelect('directories', directory.path)"
                                name="check"
                                class="fm-row-check"
                            />
                            <GIcon v-else name="folder-solid" class="fm-row-glyph fm-row-glyph--dir" />
                        </span>
                        <span class="fm-row-name"><HighlightedName v-bind:text="directory.basename" v-bind:query="highlightQuery" /></span>
                    </td>
                    <td class="fm-cell-muted">—</td>
                    <td>{{ lang.manager.table.folder }}</td>
                    <td>{{ timestampToDate(directory.timestamp) }}</td>
                    <td class="fm-permissions">{{ formatMode(directory.mode) }}</td>
                </tr>
                <tr
                    v-for="(file, index) in files"
                    v-bind:key="`f-${file.path}`"
                    v-bind:ref="(el) => registerRow(el, fileRowIndex(index))"
                    class="fm-row fm-row--file"
                    v-bind:class="{
                        'fm-row--zebra': fileRowIndex(index) % 2 === 1,
                        'fm-row--selected': checkSelect('files', file.path),
                        'fm-row--focused': focusedIndex === fileRowIndex(index),
                        'fm-row--search-current': currentMatchIndex === fileRowIndex(index),
                        'fm-row--locked': acl && file.acl === 0,
                    }"
                    tabindex="-1"
                    v-on:click="selectItem('files', file.path, $event, fileRowIndex(index))"
                    v-on:dblclick="selectAction(file)"
                    v-on:contextmenu.prevent="contextMenu(file, $event)"
                >
                    <td class="fm-content-item unselectable">
                        <span class="fm-row-icon">
                            <GIcon
                                v-if="checkSelect('files', file.path)"
                                name="check"
                                class="fm-row-check"
                            />
                            <GIcon
                                v-else
                                v-bind:name="extensionToIcon(file.extension)"
                                class="fm-row-glyph fm-row-glyph--file"
                            />
                        </span>
                        <span class="fm-row-name"><HighlightedName v-bind:text="file.basename" v-bind:query="highlightQuery" /></span>
                    </td>
                    <td>{{ bytesToHuman(file.size) }}</td>
                    <td class="fm-cell-extension">{{ file.extension || '—' }}</td>
                    <td>{{ timestampToDate(file.timestamp) }}</td>
                    <td class="fm-permissions">{{ formatMode(file.mode) }}</td>
                </tr>
                <template v-if="showSkeleton">
                    <tr
                        v-for="n in 8"
                        v-bind:key="`skel-${n}`"
                        class="fm-row fm-row--skeleton"
                        aria-hidden="true"
                    >
                        <td class="fm-content-item">
                            <span class="fm-row-icon">
                                <span class="fm-skel fm-skel--icon" />
                            </span>
                            <span
                                class="fm-skel fm-skel--name"
                                v-bind:style="{ width: `${30 + (n * 11) % 50}%` }"
                            />
                        </td>
                        <td><span class="fm-skel fm-skel--cell fm-skel--narrow" /></td>
                        <td><span class="fm-skel fm-skel--cell fm-skel--narrow" /></td>
                        <td><span class="fm-skel fm-skel--cell" /></td>
                        <td><span class="fm-skel fm-skel--cell fm-skel--narrow" /></td>
                    </tr>
                </template>
                <tr v-else-if="showError" class="fm-row fm-row--error">
                    <td colspan="5">
                        <div class="fm-empty fm-empty--error">
                            <GIcon name="warning" class="fm-empty-icon fm-empty-icon--error" />
                            <p class="fm-empty-title">{{ lang.manager.errorTitle }}</p>
                            <p class="fm-empty-hint">{{ errorMessage }}</p>
                            <button
                                type="button"
                                class="fm-empty-retry"
                                v-on:click="onRetry"
                            >
                                <GIcon name="refresh" />
                                <span>{{ lang.manager.errorRetry }}</span>
                            </button>
                        </div>
                    </td>
                </tr>
                <tr v-else-if="isEmpty" class="fm-row fm-row--empty">
                    <td colspan="5">
                        <div class="fm-empty">
                            <GIcon name="folder-open" class="fm-empty-icon" />
                            <p class="fm-empty-title">{{ lang.manager.empty }}</p>
                            <p class="fm-empty-hint">{{ lang.manager.emptyHint }}</p>
                        </div>
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>

<script setup>
/* eslint-disable no-bitwise */
import { computed, ref, nextTick, watch } from 'vue'
import { GIcon } from '@gameap/ui'
import HighlightedName from './HighlightedName.vue'
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
const { getDefaultEditor, isFileTooLarge, loadsOwnContent } = useFileEditors()

const {
    selectedDisk,
    selectedDirectory,
    files,
    directories,
    flatVisible,
    selected,
    sort,
    loading,
    error,
    selectDirectory,
    refreshDirectory,
    clearError,
    sortBy,
    selectRangeTo,
} = useManager(props.manager)

const sortSettings = computed(() => sort.value)
const acl = computed(() => settings.acl)
const isRootPath = computed(() => selectedDirectory.value === null)
const hasContent = computed(() => directories.value.length > 0 || files.value.length > 0)
const showSkeleton = computed(() => loading.value && !hasContent.value)
const showError = computed(() => !loading.value && !!error.value && !hasContent.value)
const isEmpty = computed(() => !loading.value && !error.value && !hasContent.value)
const errorMessage = computed(() => error.value?.message || lang.value.manager.errorGeneric)

async function onRetry() {
    clearError()
    await refreshDirectory()
}

const tableArea = ref(null)
const focusedIndex = ref(-1)
const rowEls = new Map()

function directoryRowIndex(index) {
    return index
}

function fileRowIndex(index) {
    return directories.value.length + index
}

function registerRow(el, index) {
    if (el) {
        rowEls.set(index, el)
    } else {
        rowEls.delete(index)
    }
}

const isActiveManager = computed(() => fm.activeManager === props.manager)
const highlightQuery = computed(() => (isActiveManager.value && fm.searchOpen ? fm.searchQuery : ''))
const currentMatchIndex = computed(() => (isActiveManager.value ? fm.currentSearchMatch : -1))

let lastMatchIndex = -1

// flush: 'post' so the rows of a fresh listing/query are already rendered and registered in rowEls
watch(
    () => [currentMatchIndex.value, fm.searchScrollTick],
    () => {
        const index = currentMatchIndex.value
        if (index < 0) return

        lastMatchIndex = index
        rowEls.get(index)?.scrollIntoView({ block: 'nearest' })
    },
    { flush: 'post' }
)

// On close, hand keyboard navigation back to the table, continuing from the last match.
watch(
    () => fm.searchOpen,
    (open, wasOpen) => {
        if (open || !wasOpen || !isActiveManager.value) return

        if (lastMatchIndex >= 0 && lastMatchIndex < flatVisible.value.length) {
            focusedIndex.value = lastMatchIndex
        }
        lastMatchIndex = -1
        nextTick(() => tableArea.value?.focus())
    }
)

function levelUp() {
    if (selectedDirectory.value) {
        const pathUp = selectedDirectory.value.split('/').slice(0, -1).join('/')
        selectDirectory(pathUp || null, true)
    }
}

function checkSelect(type, path) {
    return selected.value[type].includes(path)
}

function selectItem(type, path, event, rowIndex) {
    focusedIndex.value = rowIndex

    if (event.shiftKey) {
        event.preventDefault()
        selectRangeTo(type, path)

        return
    }

    if (event.ctrlKey || event.metaKey) {
        const alreadySelected = selected.value[type].includes(path)
        if (!alreadySelected) {
            fm.addToSelection(props.manager, { type, path })
        } else {
            fm.removeFromSelection(props.manager, { type, path })
        }

        return
    }

    fm.changeSelected(props.manager, { type, path })
}

function clearOnEmpty() {
    fm.clearSelection(props.manager)
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

function sortIcon(field) {
    if (sortSettings.value.field !== field) return 'sort-desc'

    return sortSettings.value.direction === 'down' ? 'sort-desc' : 'sort-asc'
}

function formatMode(mode) {
    if (typeof mode !== 'number') return '---------'

    const triplet = (bits) => [bits & 0o4 ? 'r' : '-', bits & 0o2 ? 'w' : '-', bits & 0o1 ? 'x' : '-'].join('')

    return triplet((mode >> 6) & 0o7) + triplet((mode >> 3) & 0o7) + triplet(mode & 0o7)
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
    if (customEditor && (!isFileTooLarge(file) || loadsOwnContent(customEditor.editor))) {
        modal.openPluginEditor({
            pluginId: customEditor.pluginId,
            editor: customEditor.editor,
            file,
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

function moveFocus(delta, withShift) {
    const total = flatVisible.value.length
    if (total === 0) return

    let next = focusedIndex.value + delta
    if (next < 0) next = 0
    if (next > total - 1) next = total - 1
    focusedIndex.value = next

    const item = flatVisible.value[next]
    if (item) {
        if (withShift) {
            selectRangeTo(item.type, item.path)
        }
        nextTick(() => {
            const el = rowEls.get(next)
            if (el) {
                el.scrollIntoView({ block: 'nearest' })
                el.focus({ preventScroll: true })
            }
        })
    }
}

function activateFocused() {
    const item = flatVisible.value[focusedIndex.value]
    if (!item) return

    if (item.type === 'directories') {
        selectDirectory(item.path, true)
    } else {
        const file = files.value.find((f) => f.path === item.path)
        if (file) selectAction(file)
    }
}

function toggleFocused() {
    const item = flatVisible.value[focusedIndex.value]
    if (!item) return

    const alreadySelected = selected.value[item.type].includes(item.path)
    if (alreadySelected) {
        fm.removeFromSelection(props.manager, { type: item.type, path: item.path })
    } else {
        fm.addToSelection(props.manager, { type: item.type, path: item.path })
    }
}

function onKeyDown(event) {
    if (modal.showModal) return

    switch (event.key) {
        case 'ArrowDown':
            event.preventDefault()
            moveFocus(1, event.shiftKey)
            break
        case 'ArrowUp':
            event.preventDefault()
            moveFocus(-1, event.shiftKey)
            break
        case 'Home':
            event.preventDefault()
            moveFocus(-flatVisible.value.length, event.shiftKey)
            break
        case 'End':
            event.preventDefault()
            moveFocus(flatVisible.value.length, event.shiftKey)
            break
        case 'Enter':
            event.preventDefault()
            activateFocused()
            break
        case ' ':
            event.preventDefault()
            toggleFocused()
            break
        default:
            break
    }
}
</script>

<style lang="scss">
.fm-table-area {
    position: relative;
    height: 100%;
    outline: none;

    &:focus-visible {
        box-shadow: inset 0 0 0 2px var(--gameap-selection-outline-weak);
    }
}

.fm-table {
    @apply w-full text-left text-sm;
    border-collapse: separate;
    border-spacing: 0;
    --fm-accent: var(--gameap-stone-800);
    --fm-match-bg: var(--gameap-orange-200);
    --fm-match-text: var(--gameap-orange-900);
    --fm-match-current-bg: var(--gameap-orange-400);
    --fm-match-current-text: var(--gameap-orange-950);

    thead th {
        @apply text-left bg-surface text-secondary font-medium;
        position: sticky;
        top: 0;
        z-index: 10;
        cursor: pointer;
        padding: 0.625rem 0.75rem;
        user-select: none;
        border-bottom: 1px solid var(--gameap-border);
        box-shadow: 0 1px 0 0 rgba(0, 0, 0, 0.04);
        transition: color 120ms ease, background-color 120ms ease;

        &:hover {
            @apply bg-stone-100 dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-700)_60%,transparent)];
        }
    }

    .fm-th--active {
        @apply text-body font-semibold;
    }

    .fm-th-sort {
        margin-left: 0.4rem;
        font-size: 0.85em;
        transition: opacity 120ms ease;
    }

    .fm-th-sort--inactive {
        opacity: 0;
    }

    thead th:hover .fm-th-sort--inactive {
        opacity: 0.4;
    }

    .fm-th--name { width: 60%; }
    .fm-th--size { width: 9%; }
    .fm-th--type { width: 9%; }
    .fm-th--date { width: 16%; }
    .fm-th--perm { width: 10ch; }

    // Phones: keep name + size, the rest returns at >=768px.
    @media (max-width: 767px) {
        th:nth-child(3),
        td:nth-child(3),
        th:nth-child(4),
        td:nth-child(4),
        th:nth-child(5),
        td:nth-child(5) {
            display: none;
        }

        .fm-th--name { width: 70%; }
        .fm-th--size { width: 30%; }
    }

    td {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        padding: 0.5rem 0.75rem;
    }

    tr.fm-row {
        cursor: pointer;
        transition: background-color 120ms ease;
        @apply bg-white dark:bg-stone-900;
        // keep scrolled-to rows clear of the sticky thead
        scroll-margin-top: 2.75rem;
        scroll-margin-bottom: 0.25rem;

        &.fm-row--zebra {
            @apply bg-stone-50 dark:bg-stone-800;
        }

        &:hover {
            @apply bg-stone-100 dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-800)_60%,transparent)];
        }

        &.fm-row--zebra:hover {
            @apply bg-stone-100 dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-700)_60%,transparent)];
        }

        &.fm-row--selected,
        &.fm-row--selected.fm-row--zebra {
            @apply bg-stone-200 dark:bg-stone-600;
            box-shadow: inset 3px 0 0 0 var(--fm-accent);
        }

        &.fm-row--selected:hover,
        &.fm-row--selected.fm-row--zebra:hover {
            @apply bg-stone-300 dark:bg-stone-500;
        }

        &.fm-row--focused {
            outline: 2px solid var(--gameap-selection-outline);
            outline-offset: -2px;
        }

        &.fm-row--search-current,
        &.fm-row--search-current.fm-row--zebra {
            background-color: var(--gameap-warning-soft);
            box-shadow: inset 3px 0 0 0 var(--gameap-warning);
        }
    }

    tr.fm-row--up {
        @apply text-muted italic;
    }

    tr.fm-row--locked {
        @apply text-faint;
    }

    tr.fm-row--empty {
        background: transparent !important;
        cursor: default;
    }

    tr.fm-row--empty:hover {
        background: transparent !important;
    }

    .fm-row-icon {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 1.5rem;
        height: 1.5rem;
        margin-right: 0.5rem;
        vertical-align: middle;
    }

    .fm-row-glyph--dir {
        @apply text-secondary;
    }

    .fm-row-glyph--file {
        @apply text-faint;
    }

    .fm-row-check {
        @apply text-body;
    }

    .fm-row-name {
        vertical-align: middle;
    }

    .fm-name-match {
        background-color: var(--fm-match-bg);
        color: var(--fm-match-text);
        border-radius: 2px;
        padding: 0 1px;
    }

    tr.fm-row--search-current .fm-name-match {
        background-color: var(--fm-match-current-bg);
        color: var(--fm-match-current-text);
    }

    .fm-cell-muted {
        @apply text-faint;
    }

    .fm-cell-extension {
        @apply text-muted uppercase tracking-wide;
        font-size: 0.78em;
    }

    .fm-permissions {
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        font-size: 0.8em;
        @apply text-muted;
    }

    .fm-content-item {
        cursor: pointer;
    }

    .fm-empty {
        @apply flex flex-col items-center justify-center text-center py-12 px-4 text-muted;
    }

    .fm-empty-icon {
        @apply text-5xl text-faint mb-4;
    }

    .fm-empty-icon--error {
        @apply text-danger;
    }

    .fm-empty-title {
        @apply text-base font-medium text-secondary mb-1;
    }

    .fm-empty-hint {
        @apply text-xs text-faint;
    }

    .fm-empty-retry {
        @apply mt-3 inline-flex items-center gap-2 px-3 py-1.5 rounded-md
               bg-stone-800 dark:bg-stone-200 text-white dark:text-stone-900
               text-sm font-medium transition-colors;
    }

    .fm-empty-retry:hover {
        @apply bg-stone-900 dark:bg-stone-100;
    }

    .fm-empty-retry:focus-visible {
        outline: 2px solid var(--gameap-selection-outline);
        outline-offset: 2px;
    }

    tr.fm-row--skeleton,
    tr.fm-row--skeleton:hover,
    tr.fm-row--error,
    tr.fm-row--error:hover {
        background: transparent !important;
        cursor: default;
    }

    .fm-skel {
        display: inline-block;
        background: var(--gameap-stone-200);
        border-radius: 4px;
        vertical-align: middle;
        animation: fm-skel-pulse 1.4s ease-in-out infinite;
    }

    .fm-skel--icon {
        width: 1.1rem;
        height: 1.1rem;
    }

    .fm-skel--name {
        height: 0.85rem;
    }

    .fm-skel--cell {
        height: 0.7rem;
        width: 60%;
    }

    .fm-skel--narrow {
        width: 35%;
    }
}

.dark .fm-table .fm-skel {
    background: var(--gameap-stone-700);
}

.dark .fm-table {
    --fm-accent: var(--gameap-stone-200);
    --fm-match-bg: color-mix(in srgb, var(--gameap-orange-500) 40%, transparent);
    --fm-match-text: var(--gameap-orange-50);
    --fm-match-current-bg: var(--gameap-orange-500);
    --fm-match-current-text: var(--gameap-orange-950);
}

@keyframes fm-skel-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.45; }
}
</style>
