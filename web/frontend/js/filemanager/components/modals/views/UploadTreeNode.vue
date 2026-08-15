<template>
    <div class="upload-tree-node">
        <div
            v-for="dir in subDirs"
            :key="dir.relPath"
            class="my-1"
        >
            <div
                class="flex items-center gap-2 cursor-pointer select-none py-1 px-2 rounded hover:bg-surface-hover"
                @click="messages.toggleDirExpanded(dir.relPath)"
            >
                <GIcon :name="dir.expanded ? 'chevron-down' : 'chevron-right'" class="text-xs text-muted w-3" />
                <GIcon :name="dir.expanded ? 'folder-open' : 'folder'" class="text-muted" />
                <span class="font-medium truncate flex-1" :title="dir.name">{{ dir.name }}</span>
                <n-tag v-if="dir.conflict === 'merge'" size="tiny" type="warning" round>
                    {{ lang.modal.upload.review.merge }}
                </n-tag>
                <n-tag v-else-if="dir.conflict === 'file-vs-dir'" size="tiny" type="error" round>
                    {{ lang.modal.upload.errors.dir_vs_file }}
                </n-tag>
                <span class="text-xs text-muted shrink-0">
                    {{ dir.completed }}/{{ dir.files }}
                </span>
                <span class="text-xs text-muted shrink-0 w-16 text-right">
                    {{ bytesToHuman(dir.size) }}
                </span>
            </div>
            <div v-if="dir.expanded" class="pl-5 border-l ml-3">
                <UploadTreeNode :dir-path="dir.relPath" :is-review="isReview" />
            </div>
        </div>
        <div
            v-for="file in directFiles"
            :key="file.index"
            class="grid grid-cols-12 items-center gap-2 py-1 px-2 rounded hover:bg-surface-hover"
        >
            <div class="col-span-12 sm:col-span-5 flex items-center gap-2 min-w-0">
                <GIcon :name="phaseIcon(file)" :class="phaseIconClass(file)" />
                <span class="truncate text-sm" :title="file.name">{{ effectiveName(file) }}</span>
            </div>
            <div class="col-span-4 sm:col-span-2 text-xs text-muted truncate text-right">
                {{ bytesToHuman(file.size) }}
            </div>
            <div class="col-span-5 sm:col-span-3">
                <template v-if="isReview && file.conflict === 'file'">
                    <n-select
                        :value="file.action"
                        :options="actionOptions"
                        size="tiny"
                        @update:value="(action) => onAction(file, action)"
                    />
                </template>
                <template v-else-if="isReview && file.conflict === 'dir-vs-file'">
                    <n-tag type="error" size="small" round>{{ lang.modal.upload.errors.dir_vs_file }}</n-tag>
                </template>
                <template v-else-if="!isReview">
                    <n-progress
                        type="line"
                        :percentage="filePercent(file)"
                        :show-indicator="false"
                        :height="6"
                        :border-radius="3"
                        :status="progressStatus(file)"
                        :processing="file.phase === 'hashing' || file.phase === 'uploading' || file.phase === 'completing'"
                    />
                </template>
            </div>
            <div class="col-span-3 sm:col-span-2 flex items-center justify-end gap-1">
                <n-tag v-if="phaseLabel(file)" :type="phaseTagType(file)" size="tiny" round :title="errorTitle(file)">
                    {{ phaseLabel(file) }}
                </n-tag>
            </div>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { NTag, NProgress, NSelect } from 'naive-ui'
import { useMessagesStore } from '../../../stores/useMessagesStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useHelper } from '../../../composables/useHelper.js'

const props = defineProps({
    dirPath: { type: String, default: '' },
    isReview: { type: Boolean, default: false },
})

const messages = useMessagesStore()
const { lang } = useTranslate()
const { bytesToHuman } = useHelper()

const directFiles = computed(() =>
    messages.uploadProgress.files.filter((f) => f.dirPath === props.dirPath),
)

const subDirs = computed(() => {
    const all = messages.uploadProgress.dirs
    const out = []
    const prefix = props.dirPath ? `${props.dirPath}/` : ''
    for (const key of Object.keys(all)) {
        if (key === '') continue
        if (props.dirPath === '' && !key.includes('/')) {
            out.push(all[key])
        } else if (props.dirPath !== '' && key.startsWith(prefix)) {
            const tail = key.slice(prefix.length)
            if (tail && !tail.includes('/')) out.push(all[key])
        }
    }
    out.sort((a, b) => a.name.localeCompare(b.name))

    return out
})

const actionOptions = computed(() => [
    { label: lang.value.modal.upload.actions.overwrite, value: 'overwrite' },
    { label: lang.value.modal.upload.actions.skip, value: 'skip' },
    { label: lang.value.modal.upload.actions.rename, value: 'rename' },
])

function onAction(file, action) {
    messages.setFileAction({ index: file.index, action })
}

function effectiveName(file) {
    if (file.action === 'rename' && file.renamedTo) return `${file.name} → ${file.renamedTo}`
    if (file.action === 'skip' && (file.conflict === 'file' || file.conflict === 'dir-vs-file')) {
        return `${file.name} (${lang.value.modal.upload.actions.skip})`
    }

    return file.name
}

function filePercent(file) {
    if (!file.size) return 0
    return Math.min(Math.round((file.loaded * 100) / file.size), 100)
}

function phaseLabel(file) {
    const phases = lang.value.modal.upload.phase || {}
    if (file.phase === 'error') {
        const errors = lang.value.modal.upload.errors || {}

        return errors[file.error] || errors.unknown || phases.error
    }
    if (file.phase === 'skipped') return lang.value.modal.upload.actions.skip
    if (file.phase === 'pending') return ''

    return phases[file.phase] || ''
}

function errorTitle(file) {
    if (file.phase === 'error' && file.errorDetail) return file.errorDetail

    return undefined
}

function phaseIcon(file) {
    if (file.phase === 'error') return 'close'
    if (file.phase === 'done') return 'check'
    if (file.phase === 'skipped') return 'pause'
    if (file.phase === 'hashing') return 'loading'
    if (file.phase === 'completing') return 'loading'
    if (file.phase === 'uploading') return 'upload'

    return 'file'
}

function phaseIconClass(file) {
    if (file.phase === 'error') return 'text-danger'
    if (file.phase === 'done') return 'text-success'
    if (file.phase === 'skipped') return 'text-faint'
    if (file.phase === 'hashing' || file.phase === 'completing') return 'text-info'
    if (file.phase === 'uploading') return 'text-info'

    return 'text-muted'
}

function phaseTagType(file) {
    if (file.phase === 'error') return 'error'
    if (file.phase === 'done') return 'success'
    if (file.phase === 'skipped') return 'default'

    return 'default'
}

function progressStatus(file) {
    if (file.phase === 'error') return 'error'
    if (file.phase === 'done') return 'success'

    return 'default'
}
</script>
