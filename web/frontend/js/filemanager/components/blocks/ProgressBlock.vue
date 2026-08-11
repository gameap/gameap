<template>
    <div v-if="visible" class="fm-progress-block">
        <div
            v-for="op in operations"
            :key="op.id"
            class="flex items-center gap-3 px-3 py-2"
        >
            <GIcon :name="op.type === 'archive' ? 'file-zipper' : 'box-open'" class="text-sky-500 shrink-0" />
            <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between mb-1">
                    <span class="text-xs truncate" :title="op.currentEntry || op.label">
                        {{ operationLabel(op) }}
                    </span>
                    <span v-if="operationDeterminate(op)" class="text-xs text-stone-500 shrink-0 ml-2">
                        {{ operationPercent(op) }}%
                    </span>
                </div>
                <n-progress
                    type="line"
                    :percentage="op.status === 'error' ? 100 : operationPercent(op)"
                    :status="op.status === 'error' ? 'error' : 'default'"
                    :show-indicator="false"
                    :height="6"
                    :border-radius="3"
                    :processing="['pending', 'running', 'cancelling'].includes(op.status)"
                    :indeterminate="!operationDeterminate(op) && ['pending', 'running', 'cancelling'].includes(op.status)"
                />
            </div>
            <button
                v-if="['pending', 'running', 'cancelling'].includes(op.status)"
                type="button"
                class="text-xs text-stone-500 hover:text-stone-800 dark:hover:text-stone-200 px-2 py-1 shrink-0"
                :disabled="op.status === 'cancelling'"
                @click="ops.cancelOperation(op.id)"
            >
                {{ op.status === 'cancelling' ? lang.progress.operationCancelling : lang.btn.cancel }}
            </button>
            <button
                v-else-if="op.status === 'error' || op.status === 'stale'"
                type="button"
                class="text-xs text-stone-500 hover:text-stone-800 dark:hover:text-stone-200 px-2 py-1 shrink-0"
                @click="ops.remove(op.id)"
            >
                {{ lang.btn.close }}
            </button>
        </div>
        <div v-if="archiveVisible" class="flex items-center gap-3 px-3 py-2">
            <GIcon :name="archiveIcon" class="text-sky-500 shrink-0" />
            <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between mb-1">
                    <span class="text-xs truncate" :title="archiveTooltip">{{ archiveLabel }}</span>
                    <span v-if="archiveDeterminate" class="text-xs text-stone-500 shrink-0 ml-2">
                        {{ archivePercent }}%
                    </span>
                </div>
                <n-progress
                    type="line"
                    :percentage="archiveErrored ? 100 : (archiveDeterminate ? archivePercent : 0)"
                    :status="archiveErrored ? 'error' : 'default'"
                    :show-indicator="false"
                    :height="6"
                    :border-radius="3"
                    :processing="!archiveErrored && (!archiveDeterminate || archive.status !== 'completed')"
                    :indeterminate="!archiveErrored && !archiveDeterminate"
                />
            </div>
            <button
                v-if="archive.status !== 'completed' && archive.status !== 'error'"
                type="button"
                class="text-xs text-stone-500 hover:text-stone-800 dark:hover:text-stone-200 px-2 py-1 shrink-0"
                @click="cancelArchive"
            >
                {{ lang.btn.cancel }}
            </button>
            <button
                v-else-if="archive.status === 'error'"
                type="button"
                class="text-xs text-stone-500 hover:text-stone-800 dark:hover:text-stone-200 px-2 py-1 shrink-0"
                @click="dismissArchive"
            >
                {{ lang.btn.close }}
            </button>
        </div>
        <div v-else class="flex items-center gap-3 px-3 py-2">
            <GIcon name="download" class="text-sky-500 shrink-0" />
            <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between mb-1">
                    <span class="text-xs truncate" :title="label">{{ label }}</span>
                    <span class="text-xs text-stone-500 shrink-0 ml-2">{{ progressBar }}%</span>
                </div>
                <n-progress
                    type="line"
                    :percentage="progressBar"
                    :show-indicator="false"
                    :height="6"
                    :border-radius="3"
                    processing
                />
            </div>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { useMessagesStore } from '../../stores/useMessagesStore.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useArchiveOperationsStore } from '../../stores/useArchiveOperationsStore.js'
import { useTranslate } from '../../composables/useTranslate.js'

const messages = useMessagesStore()
const fm = useFileManagerStore()
const ops = useArchiveOperationsStore()
const { lang } = useTranslate()

const operations = computed(() => ops.visibleOperations)

function operationDeterminate(op) {
    return op.bytesTotal > 0 || op.filesTotal > 0
}

function operationPercent(op) {
    if (op.status === 'completed') return 100
    if (op.bytesTotal > 0) return Math.min(100, Math.round((op.bytesProcessed / op.bytesTotal) * 100))
    if (op.filesTotal > 0) return Math.min(100, Math.round((op.filesProcessed / op.filesTotal) * 100))

    return 0
}

function operationLabel(op) {
    const phrases = lang.value.progress

    if (op.status === 'stale') {
        return `${op.label} — ${phrases.operationConnectionLost}`
    }
    if (op.status === 'error') {
        const message = lang.value.response[op.error] ?? op.error ?? phrases.operationFailed
        return `${op.label} — ${message}`
    }

    const base = op.type === 'archive' ? phrases.archiveCreating : phrases.archiveExtracting
    let label = `${base}: ${op.label}`
    if (op.filesTotal > 0) {
        label += ` — ${phrases.operationFiles
            .replace('{processed}', String(op.filesProcessed))
            .replace('{total}', String(op.filesTotal))}`
    }

    return label
}

const progressBar = computed(() => messages.actionProgress)
const label = computed(() => messages.progressLabel)

const archive = computed(() => messages.archiveDownload)
const archiveVisible = computed(() => archive.value.status !== 'idle')
const archiveKind = computed(() => archive.value.kind || 'archive')
const archiveIcon = computed(() => (archiveKind.value === 'file' ? 'download' : 'folder-download'))
const archiveDeterminate = computed(() => archive.value.total > 0)
const archiveErrored = computed(() => archive.value.status === 'error')
const archivePercent = computed(() => {
    if (!archiveDeterminate.value) return 0

    return Math.min(100, Math.round((archive.value.loaded / archive.value.total) * 100))
})

const ERROR_DETAIL_MAX = 100

const truncate = (s, max) => {
    if (!s) return ''
    if (s.length <= max) return s

    return `${s.slice(0, max - 1).trimEnd()}…`
}

const archiveErrorBase = computed(() => {
    const phrases = lang.value.progress || {}
    if (archive.value.kind === 'file') {
        return phrases.downloadError || phrases.archiveError || 'Download failed'
    }

    return phrases.archiveError || 'Archive download failed'
})

const archiveErrorDetail = computed(() => {
    const ad = archive.value
    if (ad.status !== 'error') return ''
    const message = ad.error && ad.error.message ? String(ad.error.message) : ''

    return message
})

const archiveLabel = computed(() => {
    const ad = archive.value
    const phrases = lang.value.progress || {}
    const isFile = ad.kind === 'file'

    if (ad.status === 'error') {
        const detail = archiveErrorDetail.value
        if (!detail) return archiveErrorBase.value

        return `${archiveErrorBase.value}: ${truncate(detail, ERROR_DETAIL_MAX)}`
    }
    if (ad.status === 'preparing') {
        if (isFile) return phrases.preparingDownload || phrases.downloading || 'Preparing download...'

        return phrases.preparingArchive || 'Preparing archive...'
    }
    if (ad.status === 'completed') {
        return ad.filename
    }

    if (isFile) {
        const baseDl = phrases.downloading || 'Downloading'

        return ad.filename ? `${baseDl}: ${ad.filename}` : baseDl
    }

    const base = phrases.downloadingArchive || 'Downloading archive'
    if (ad.totalFiles > 0 && phrases.archiveFiles) {
        const filesLabel = phrases.archiveFiles.replace('{count}', String(ad.totalFiles))

        return `${base}: ${ad.filename} — ${filesLabel}`
    }

    return ad.filename ? `${base}: ${ad.filename}` : base
})

const visible = computed(
    () => archiveVisible.value || operations.value.length > 0 || progressBar.value > 0 || label.value,
)

const archiveTooltip = computed(() => {
    if (archive.value.status === 'error' && archiveErrorDetail.value) {
        return `${archiveErrorBase.value}: ${archiveErrorDetail.value}`
    }

    return archiveLabel.value
})

function cancelArchive() {
    fm.cancelDirectoryDownload()
}

function dismissArchive() {
    messages.clearArchiveDownload()
}
</script>

<style lang="scss">
.fm-progress-block {
    @apply border-t bg-stone-50 dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-800)_50%,transparent)];
    flex: 0 0 auto;
}
</style>
