<template>
    <div class="fm-info-block">
        <div class="fm-info-left" v-if="selectedCount === 0">
            <span class="fm-info-pill">
                <GIcon name="folder" class="fm-info-pill-icon" />
                <span>{{ directoriesCount }}</span>
            </span>
            <span class="fm-info-pill">
                <GIcon name="file" class="fm-info-pill-icon" />
                <span>{{ filesCount }}</span>
            </span>
            <span class="fm-info-pill">
                <GIcon name="hard-drive" class="fm-info-pill-icon" />
                <span>{{ filesSize }}</span>
            </span>
        </div>
        <div class="fm-info-left fm-info-left--selected" v-else>
            <span class="fm-info-pill fm-info-pill--accent">
                <GIcon name="check" class="fm-info-pill-icon" />
                <span>{{ selectedCount }}</span>
                <span class="fm-info-pill-label">{{ lang.info.selected }}</span>
            </span>
            <span class="fm-info-pill" v-if="selectedDirsCount > 0">
                <GIcon name="folder" class="fm-info-pill-icon" />
                <span>{{ selectedDirsCount }}</span>
            </span>
            <span class="fm-info-pill" v-if="selectedFilesCount > 0">
                <GIcon name="file" class="fm-info-pill-icon" />
                <span>{{ selectedFilesCount }}</span>
            </span>
            <span class="fm-info-pill" v-if="selectedFilesCount > 0">
                <GIcon name="hard-drive" class="fm-info-pill-icon" />
                <span>{{ selectedFilesSize }}</span>
            </span>
            <button
                type="button"
                class="fm-info-clear"
                v-bind:title="`${lang.info.clearSelection} (${lang.hint.esc})`"
                v-on:click="clearSelection"
            >
                <GIcon name="close" />
                <span>{{ lang.info.clearSelection }}</span>
            </button>
        </div>

        <div class="fm-info-right">
            <span v-show="loadingSpinner" class="fm-info-icon-btn fm-info-icon-btn--passive" role="status">
                <GIcon name="spinner" />
            </span>
            <button
                type="button"
                class="fm-info-icon-btn"
                v-show="clipboardType"
                v-bind:title="`${lang.clipboard.title} — ${lang.clipboard[clipboardType] || ''}`"
                v-on:click="showModal('ClipboardModal')"
            >
                <GIcon name="clipboard" />
            </button>
            <button
                type="button"
                class="fm-info-icon-btn"
                v-bind:class="hasErrors ? 'fm-info-icon-btn--error' : 'fm-info-icon-btn--ok'"
                v-bind:title="lang.modal.status.title"
                v-on:click="showModal('StatusModal')"
            >
                <GIcon name="info" />
            </button>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useMessagesStore } from '../../stores/useMessagesStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useTranslate } from '../../composables/useTranslate.js'
import { useHelper } from '../../composables/useHelper.js'

const fm = useFileManagerStore()
const messages = useMessagesStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { bytesToHuman } = useHelper()

const activeManager = computed(() => fm.activeManager)
const hasErrors = computed(() => !!messages.errors.length)
const filesCount = computed(() => fm.getFilesCount(activeManager.value))
const directoriesCount = computed(() => fm.getDirectoriesCount(activeManager.value))
const filesSize = computed(() => bytesToHuman(fm.getFilesSize(activeManager.value)))
const selectedCount = computed(() => fm.getSelectedCount(activeManager.value))
const selectedFilesSize = computed(() => bytesToHuman(fm.getSelectedFilesSize(activeManager.value)))
const selectedDirsCount = computed(() => {
    const manager = fm.getManager(activeManager.value)

    return manager.selected.directories.length
})
const selectedFilesCount = computed(() => {
    const manager = fm.getManager(activeManager.value)

    return manager.selected.files.length
})
const clipboardType = computed(() => fm.clipboard.type)
const loadingSpinner = computed(() => messages.loading)

function showModal(modalName) {
    modal.setModalState({ modalName, show: true })
}

function clearSelection() {
    fm.clearSelection(activeManager.value)
}
</script>

<style lang="scss">
.fm-info-block {
    @apply flex items-center justify-between gap-4 pt-2 pb-1 px-1
        border-t text-xs text-stone-600 dark:text-stone-400;
    flex: 0 0 auto;
}

.fm-info-block.hidden {
    display: none;
}

.fm-info-left {
    @apply flex items-center gap-2 flex-wrap min-w-0;
}

.fm-info-left--selected {
    @apply text-stone-800 dark:text-stone-100;
}

.fm-info-pill {
    @apply inline-flex items-center gap-1.5 px-2 py-0.5 rounded
        bg-stone-100 dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-800)_60%,transparent)] text-secondary;
    font-variant-numeric: tabular-nums;
}

.fm-info-pill--accent {
    @apply bg-stone-200 dark:bg-stone-600 text-stone-900 dark:text-stone-50 font-semibold;
}

.fm-info-pill-icon {
    @apply text-faint;
    font-size: 0.85em;
}

.fm-info-pill--accent .fm-info-pill-icon {
    @apply text-stone-700 dark:text-stone-200;
}

.fm-info-pill-label {
    @apply font-normal text-secondary text-[0.7rem] uppercase tracking-wide;
}

.fm-info-clear {
    @apply inline-flex items-center gap-1 px-2 py-0.5 rounded
        text-muted
        hover:bg-stone-200 dark:hover:bg-stone-700 hover:text-stone-800 dark:hover:text-stone-200
        transition-colors duration-100;
    cursor: pointer;
}

.fm-info-right {
    @apply flex items-center gap-1 flex-shrink-0;
}

.fm-info-icon-btn {
    @apply inline-flex items-center justify-center w-6 h-6 rounded
        text-muted
        hover:bg-stone-100 dark:hover:bg-stone-700 hover:text-stone-700 dark:hover:text-stone-200
        transition-colors duration-100;
    cursor: pointer;
}

.fm-info-icon-btn--ok {
    @apply text-muted;
}

.fm-info-icon-btn--error {
    @apply text-red-500 dark:text-red-400;
}

.fm-info-icon-btn--passive {
    cursor: default;
}

@media (max-width: 640px) {
    .fm-info-block {
        display: none;
    }
}

@media (max-height: 500px) {
    .fm-info-block {
        display: none !important;
    }
}
</style>
