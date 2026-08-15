<template>
    <div class="fm-navbar">
        <div class="fm-toolbar">
            <div class="fm-toolbar-group" role="group">
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:title="`${lang.btn.refresh} (${lang.hint.f5})`"
                    v-on:click="refreshAll"
                >
                    <GIcon name="refresh" />
                </button>
            </div>

            <div class="fm-toolbar-group" role="group">
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:title="lang.btn.file"
                    v-on:click="showModal('NewFileModal')"
                >
                    <GIcon name="file" />
                </button>
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:title="lang.btn.folder"
                    v-on:click="showModal('NewFolderModal')"
                >
                    <GIcon name="folder" />
                </button>
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:disabled="uploading"
                    v-bind:title="lang.btn.upload"
                    v-on:click="showModal('UploadModal')"
                >
                    <GIcon name="upload" />
                </button>
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:disabled="archiveDownloading"
                    v-bind:title="lang.btn.downloadDir"
                    v-on:click="handleDownloadDirectory"
                >
                    <GIcon name="folder-download" />
                </button>
            </div>

            <div class="fm-toolbar-group" role="group">
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:disabled="!isAnyItemSelected"
                    v-bind:title="lang.btn.copy"
                    v-on:click="handleToClipboard('copy')"
                >
                    <GIcon name="copy" />
                </button>
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:disabled="!isAnyItemSelected"
                    v-bind:title="lang.btn.cut"
                    v-on:click="handleToClipboard('cut')"
                >
                    <GIcon name="cut" />
                </button>
                <button
                    type="button"
                    class="fm-tool-btn"
                    v-bind:disabled="!clipboardType"
                    v-bind:title="lang.btn.paste"
                    v-on:click="handlePaste"
                >
                    <GIcon name="paste" />
                </button>
            </div>

            <div class="fm-toolbar-spacer" />

            <div class="fm-toolbar-group" role="group">
                <button
                    type="button"
                    class="fm-tool-btn fm-tool-btn--danger"
                    v-bind:disabled="!isAnyItemSelected"
                    v-bind:title="`${lang.btn.delete} (${lang.hint.del})`"
                    v-on:click="showModal('DeleteModal')"
                >
                    <GIcon name="delete" />
                </button>
            </div>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { notification } from '@/parts/dialogs.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useMessagesStore } from '../../stores/useMessagesStore.js'
import { useModalStore } from '../../stores/useModalStore.js'
import { useTranslate } from '../../composables/useTranslate.js'

const fm = useFileManagerStore()
const messages = useMessagesStore()
const modal = useModalStore()
const { lang } = useTranslate()

const activeManager = computed(() => fm.activeManager)

const isAnyItemSelected = computed(() => {
    const manager = fm.getManager(activeManager.value)

    return manager.selected.files.length > 0 || manager.selected.directories.length > 0
})

const uploading = computed(() => messages.actionProgress > 0)

const archiveDownloading = computed(() => {
    const status = messages.archiveDownload?.status

    return status === 'preparing' || status === 'downloading'
})

const clipboardType = computed(() => fm.clipboard.type)

function refreshAll() {
    fm.refreshAll()
}

function handleToClipboard(type) {
    fm.toClipboard(type)

    if (type === 'cut') {
        notification({ content: lang.value.notifications.cutToClipboard, type: 'success' })
    } else if (type === 'copy') {
        notification({ content: lang.value.notifications.copyToClipboard, type: 'success' })
    }
}

function handlePaste() {
    fm.paste()
}

function handleDownloadDirectory() {
    fm.downloadCurrentDirectory()
}

function showModal(modalName) {
    modal.setModalState({ modalName, show: true })
}
</script>

<style lang="scss">
.fm-navbar {
    flex: 0 0 auto;
    margin-bottom: 0.6rem;
}

.fm-toolbar {
    @apply flex flex-wrap items-center gap-2;
}

/* Segmented button group: shared border + dividers, same shell as GBreadcrumbs
   (border token, stone surface) so the toolbar reads as controls, not icons. */
.fm-toolbar-group {
    @apply inline-flex items-stretch overflow-hidden rounded-lg border bg-surface shadow-sm;
}

.fm-toolbar-spacer {
    flex: 1 1 auto;
}

.fm-tool-btn {
    @apply inline-flex items-center justify-center px-2.5 py-1.5 text-sm
        text-secondary
        transition-colors duration-100
        border-r;
    min-width: 2.25rem;

    &:last-child {
        border-right: none;
    }

    &:hover:not(:disabled) {
        @apply bg-stone-100 dark:bg-stone-700 text-body;
    }

    &:focus-visible {
        @apply outline-none ring-2 ring-stone-500 ring-offset-0;
        z-index: 1;
    }

    &:disabled {
        @apply text-faint cursor-not-allowed;
    }
}

.fm-tool-btn--toggled {
    @apply bg-stone-200 dark:bg-stone-600 text-stone-900 dark:text-stone-50;
}

.fm-tool-btn--toggled:hover:not(:disabled) {
    @apply bg-stone-300 dark:bg-stone-500 text-stone-900 dark:text-white;
}

.fm-tool-btn--danger:hover:not(:disabled) {
    @apply bg-danger-soft text-danger;
}
</style>
