<template>
    <div class="fm-modal-upload flex flex-col gap-3 min-h-0">
        <template v-if="status === 'idle'">
            <n-upload
                multiple
                directory-dnd
                :default-upload="false"
                :show-file-list="false"
                :on-before-upload="onBeforeUpload"
            >
                <n-upload-dragger>
                    <div class="flex flex-col items-center gap-2 py-6">
                        <GIcon name="upload" class="text-4xl text-faint" />
                        <p class="text-secondary font-medium">
                            {{ lang.modal.upload.dragger.text }}
                        </p>
                        <p class="text-sm text-muted">
                            {{ lang.modal.upload.dragger.hint }}
                        </p>
                    </div>
                </n-upload-dragger>
            </n-upload>
            <div class="text-center">
                <GButton color="black" @click="openFolderPicker">
                    <GIcon name="folder" class="mr-1" />
                    {{ lang.btn.uploadDir }}
                </GButton>
                <input
                    ref="folderInputRef"
                    type="file"
                    webkitdirectory
                    multiple
                    class="hidden"
                    @change="onFolderInputChange"
                />
            </div>
            <p v-if="!hasPending" class="text-muted text-center text-sm">
                {{ lang.modal.upload.noSelected }}
            </p>
        </template>

        <div v-if="status === 'preflight'" class="py-8 flex flex-col items-center gap-3 text-muted">
            <GIcon name="loading" class="text-3xl text-info" />
            <p>{{ lang.modal.upload.detecting }}</p>
        </div>

        <template v-if="status === 'review'">
            <n-alert
                v-if="hasConflicts"
                type="warning"
                :show-icon="true"
                :title="lang.modal.upload.review.conflictsHeader"
                class="shrink-0"
            >
                <div class="flex flex-wrap items-center gap-2">
                    <span>{{ lang.modal.upload.review.defaultAction }}</span>
                    <div class="w-48">
                        <n-select
                            :value="defaultAction"
                            :options="defaultActionOptions"
                            size="small"
                            @update:value="onDefaultAction"
                        />
                    </div>
                </div>
            </n-alert>
            <div v-if="!isSingleFile" class="flex flex-wrap gap-x-4 gap-y-1 text-sm shrink-0">
                <span>
                    <span class="text-muted">{{ lang.modal.upload.summaryFiles }}</span>
                    {{ totals.files }}
                </span>
                <span>
                    <span class="text-muted">{{ lang.modal.upload.summaryDirs }}</span>
                    {{ dirCount }}
                </span>
                <span>
                    <span class="text-muted">{{ lang.modal.upload.size }}</span>
                    {{ bytesToHuman(totals.bytes) }}
                </span>
            </div>
            <div class="min-h-0 overflow-auto border rounded">
                <UploadTreeNode :is-review="true" />
            </div>
        </template>

        <template v-if="status === 'mkdir' || status === 'uploading'">
            <div v-if="!isSingleFile" class="shrink-0">
                <div class="flex items-center justify-between gap-2 mb-1 text-sm">
                    <span>
                        {{ status === 'mkdir' ? lang.modal.upload.creatingDirs : lang.modal.upload.phase.uploading }}
                    </span>
                    <span class="text-xs text-muted tabular-nums">
                        {{ totals.completedFiles }}/{{ totals.files }}
                        ·
                        {{ bytesToHuman(totals.loadedBytes) }} / {{ bytesToHuman(totals.bytes) }}
                    </span>
                </div>
                <n-progress
                    type="line"
                    :percentage="overallPercent"
                    :show-indicator="false"
                    :height="8"
                    :border-radius="4"
                    processing
                />
            </div>
            <div class="min-h-0 overflow-auto border rounded">
                <UploadTreeNode :is-review="false" />
            </div>
        </template>

        <template v-if="status === 'completed' || status === 'partial' || status === 'cancelled'">
            <n-alert
                :type="resultAlertType"
                :show-icon="true"
                :title="isSingleFile ? undefined : resultTitle"
                class="shrink-0"
            >
                {{ isSingleFile ? resultTitle : resultMessage }}
            </n-alert>
            <div class="min-h-0 overflow-auto border rounded">
                <UploadTreeNode :is-review="false" />
            </div>
        </template>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { GIcon } from '@gameap/ui'
import { NAlert, NUpload, NUploadDragger, NSelect, NProgress } from 'naive-ui'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useMessagesStore } from '../../../stores/useMessagesStore.js'
import { useModalStore } from '../../../stores/useModalStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useHelper } from '../../../composables/useHelper.js'
import { useModal } from '../../../composables/useModal.js'
import { entriesFromFileList, entriesFromUploadFiles } from '../../../composables/useDropZone.js'
import UploadTreeNode from './UploadTreeNode.vue'

const fm = useFileManagerStore()
const messages = useMessagesStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { bytesToHuman } = useHelper()
const { hideModal } = useModal()

const folderInputRef = ref(null)
const pendingDrop = ref({ entries: [], emptyDirs: [] })
let pickedBatch = []

const status = computed(() => messages.uploadProgress.status)
const totals = computed(() => messages.uploadProgress.totals)
const defaultAction = computed(() => messages.uploadProgress.defaultAction)
const hasConflicts = computed(() => messages.uploadProgress.hasConflicts)
const dirCount = computed(() => Object.keys(messages.uploadProgress.dirs).filter((k) => k !== '').length)
const isSingleFile = computed(() => totals.value.files === 1 && dirCount.value === 0 && messages.uploadProgress.emptyDirs.length === 0)
const overallPercent = computed(() => {
    const t = totals.value
    if (!t.bytes) return 0

    return Math.min(Math.round((t.loadedBytes * 100) / t.bytes), 100)
})

const canCancelUpload = computed(() => {
    const up = messages.uploadProgress
    if (!up.abortController) return false
    if (up.abortController.signal.aborted) return false

    return up.files.some(
        (f) => f.phase === 'pending' || f.phase === 'hashing' || f.phase === 'uploading',
    )
})

const hasPending = computed(() => pendingDrop.value.entries.length > 0 || pendingDrop.value.emptyDirs.length > 0)

const defaultActionOptions = computed(() => [
    { label: lang.value.modal.upload.actions.overwriteAll, value: 'overwrite' },
    { label: lang.value.modal.upload.actions.skipAll, value: 'skip' },
    { label: lang.value.modal.upload.actions.renameAll, value: 'rename' },
])

const resultAlertType = computed(() => {
    if (status.value === 'completed') return 'success'
    if (status.value === 'cancelled') return 'default'

    return 'warning'
})

const resultTitle = computed(() => {
    if (status.value === 'completed') return lang.value.modal.upload.resultSuccess
    if (status.value === 'cancelled') return lang.value.modal.upload.resultCancelled

    return lang.value.modal.upload.resultPartial
})

const resultMessage = computed(() => {
    const t = totals.value

    return lang.value.modal.upload.completed
        .replace('{success}', t.completedFiles)
        .replace('{failed}', t.failedFiles)
        .replace('{skipped}', t.skippedFiles)
})

function openFolderPicker() {
    if (folderInputRef.value) folderInputRef.value.click()
}

// n-upload emits change once per file, so a multi-file pick would start one
// preflight per file, each replacing the previous one. It calls
// on-before-upload synchronously for every file of a pick before any change
// event, so the pick is collected here and submitted once in a microtask;
// returning false keeps the files out of n-upload's own list.
function onBeforeUpload({ file }) {
    if (pickedBatch.length === 0) queueMicrotask(submitPickedBatch)
    pickedBatch.push(file)

    return false
}

function submitPickedBatch() {
    const result = entriesFromUploadFiles(pickedBatch)
    pickedBatch = []
    pendingDrop.value = { entries: result.files, emptyDirs: result.emptyDirs }
    submitPending()
}

function onFolderInputChange(event) {
    if (!event.target.files || event.target.files.length === 0) return
    const result = entriesFromFileList(event.target.files)
    pendingDrop.value = { entries: result.files, emptyDirs: result.emptyDirs }
    submitPending()
    event.target.value = ''
}

async function submitPending() {
    const { entries, emptyDirs } = pendingDrop.value
    if (entries.length === 0 && emptyDirs.length === 0) return
    pendingDrop.value = { entries: [], emptyDirs: [] }
    await fm.upload({ entries, emptyDirs })
}

function onDefaultAction(action) {
    messages.setDefaultAction(action)
}

function applyPayloadFromModal() {
    const payload = modal.consumePayload()
    if (!payload) return
    const entries = payload.entries || []
    const emptyDirs = payload.emptyDirs || []
    if (entries.length === 0 && emptyDirs.length === 0) return
    pendingDrop.value = { entries, emptyDirs }
    submitPending()
}

function close() {
    if (status.value === 'mkdir' || status.value === 'uploading') {
        const ok = window.confirm(lang.value.modal.upload.confirmCloseUploading)
        if (!ok) return
        fm.cancelUpload()
    }
    fm.clearUpload()
    hideModal()
}

function onSubmitReview() {
    fm.startUpload()
}

function onCancelAll() {
    fm.cancelUpload()
}

function onRetryFailed() {
    fm.retryFailed()
}

onMounted(() => {
    const terminal = ['completed', 'partial', 'cancelled']
    if (terminal.includes(status.value)) {
        messages.clearUploadProgress()
    }
    applyPayloadFromModal()
})

onUnmounted(() => {
    const active = ['mkdir', 'uploading']
    if (active.includes(status.value)) {
        fm.cancelUpload()
    }
    messages.clearUploadProgress()
})

watch(
    () => modal.payload,
    (val) => {
        if (val && status.value === 'idle') applyPayloadFromModal()
    },
)

defineExpose({
    footerButtons: computed(() => {
        if (status.value === 'idle') {
            return [
                {
                    label: lang.value.btn.cancel,
                    color: 'black',
                    icon: 'close',
                    action: close,
                },
            ]
        }
        if (status.value === 'preflight') {
            return [
                {
                    label: lang.value.btn.cancel,
                    color: 'black',
                    icon: 'close',
                    action: close,
                },
            ]
        }
        if (status.value === 'review') {
            return [
                {
                    label: lang.value.btn.cancel,
                    color: 'black',
                    icon: 'close',
                    action: close,
                },
                {
                    label: lang.value.btn.submit,
                    color: 'green',
                    icon: 'upload',
                    action: onSubmitReview,
                    disabled: totals.value.files === 0,
                },
            ]
        }
        if (status.value === 'mkdir' || status.value === 'uploading') {
            return [
                {
                    label: lang.value.btn.cancelAll,
                    color: 'red',
                    icon: 'stop',
                    action: onCancelAll,
                    disabled: !canCancelUpload.value,
                },
            ]
        }
        const buttons = [{
            label: lang.value.btn.close,
            color: 'black',
            icon: 'close',
            action: close,
        }]

        if (totals.value.failedFiles > 0) {
            buttons.push({
                label: lang.value.btn.retryFailed,
                color: 'orange',
                icon: 'refresh',
                action: onRetryFailed,
            })
        }

        return buttons
    }),
})
</script>
