<template>
    <div>
        <div v-if="selectedItems.length === 0" class="text-red-500 dark:text-red-400">
            {{ lang.modal.zip.noSelected }}
        </div>
        <div v-else>
            <div class="mb-3">
                <span class="block mb-1">{{ lang.modal.zip.itemsToArchive }}</span>
                <ul class="fm-zip-item-list text-sm">
                    <li v-for="item in selectedItems" :key="item.path" class="truncate" :title="item.path">
                        <GIcon :name="item.type === 'dir' ? 'folder' : 'file'" class="mr-1" />
                        {{ item.basename }}
                    </li>
                </ul>
            </div>

            <div class="mb-2">
                <label for="fm-zip-name" class="block mb-1">{{ lang.modal.zip.fieldName }}</label>
                <n-input
                    id="fm-zip-name"
                    v-model:value="name"
                    :status="nameInvalid || nameConflict ? 'error' : undefined"
                    @keyup.enter="submit"
                />
                <div v-if="nameInvalid" class="text-red-500 text-sm mt-1">
                    {{ lang.modal.zip.invalidExtension }}
                </div>
                <div v-else-if="nameConflict" class="text-red-500 text-sm mt-1">
                    {{ lang.modal.zip.fieldFeedback }}
                </div>
            </div>

            <div class="mb-2">
                <label for="fm-zip-format" class="block mb-1">{{ lang.modal.zip.format }}</label>
                <n-select
                    id="fm-zip-format"
                    :value="derivedFormat"
                    :options="formatOptions"
                    @update:value="onFormatPicked"
                />
            </div>

            <div class="mb-2">
                <n-checkbox v-model:checked="overwrite">
                    {{ lang.modal.zip.overwrite }}
                </n-checkbox>
            </div>

            <div class="mb-1">
                <button type="button" class="text-sm text-sky-600 dark:text-sky-400" @click="advanced = !advanced">
                    {{ lang.modal.zip.advanced }}
                </button>
            </div>
            <div v-if="advanced" class="mb-2">
                <label for="fm-zip-level" class="block mb-1">{{ lang.modal.zip.compressionLevel }}</label>
                <n-input-number
                    id="fm-zip-level"
                    v-model:value="compressionLevel"
                    :min="0"
                    :max="9"
                    clearable
                />
                <div class="text-xs text-stone-500 mt-1">{{ lang.modal.zip.compressionLevelHint }}</div>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, computed, inject } from 'vue'
import { NInput, NSelect, NCheckbox, NInputNumber } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import POST from '../../../http/post.js'
import { CREATE_FORMATS, deriveCreateFormat, replaceCreateSuffix } from '../../../archive.js'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../../stores/useSettingsStore.js'
import { useArchiveOperationsStore } from '../../../stores/useArchiveOperationsStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const fm = useFileManagerStore()
const settings = useSettingsStore()
const ops = useArchiveOperationsStore()
const { lang } = useTranslate()
const { hideModal } = useModal()
const archiveSocket = inject('fm-archive-socket', null)

const selectedItems = computed(() => fm.selectedItems)

function sanitizeName(value) {
    return String(value || '').replace(/[/\\]/g, '').trim()
}

function defaultName() {
    if (selectedItems.value.length === 1) {
        const item = selectedItems.value[0]
        const base = item.type === 'file' ? (item.filename || item.basename) : item.basename

        return `${sanitizeName(base) || 'archive'}.zip`
    }

    const dirBase = String(fm.selectedDirectory || '').split('/').filter(Boolean).pop()

    return `${sanitizeName(dirBase || settings.serverName) || 'archive'}.zip`
}

const name = ref(defaultName())
const overwrite = ref(false)
const advanced = ref(false)
const compressionLevel = ref(null)
const submitting = ref(false)

const formatOptions = CREATE_FORMATS.map((f) => ({ value: f.value, label: f.suffix.slice(1) }))

const derivedFormat = computed(() => deriveCreateFormat(name.value))
const nameInvalid = computed(() => sanitizeName(name.value) !== '' && derivedFormat.value === null)
const nameConflict = computed(
    () => !overwrite.value && fm.fileExist(fm.activeManager, sanitizeName(name.value)),
)
const submitDisabled = computed(
    () =>
        submitting.value ||
        selectedItems.value.length === 0 ||
        sanitizeName(name.value) === '' ||
        derivedFormat.value === null ||
        nameConflict.value,
)

function onFormatPicked(format) {
    name.value = replaceCreateSuffix(sanitizeName(name.value) || 'archive', format)
}

async function submit() {
    if (submitDisabled.value) return

    submitting.value = true

    const archiveName = sanitizeName(name.value)

    try {
        // Subscribe before starting: small archives finish faster than a
        // late-connecting socket would attach.
        if (archiveSocket) {
            await archiveSocket.connectAndWait()
        }

        const response = await POST.archive({
            disk: fm.selectedDisk,
            path: fm.selectedDirectory || '',
            name: archiveName,
            format: derivedFormat.value,
            sources: selectedItems.value.map((item) => item.path),
            ...(compressionLevel.value !== null ? { compression_level: compressionLevel.value } : {}),
            overwrite: overwrite.value,
        })

        ops.add({
            id: response.data.operation_id,
            type: 'archive',
            label: archiveName,
            disk: fm.selectedDisk,
        })
        hideModal()
    } catch (e) {
        // The global interceptor already surfaced the error; keep the modal open.
    } finally {
        submitting.value = false
    }
}

defineExpose({
    footerButtons: computed(() => [
        {
            label: lang.value.btn.submit,
            color: 'green',
            icon: 'file-zipper',
            action: submit,
            disabled: submitDisabled.value,
        },
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>

<style lang="scss">
.fm-zip-item-list {
    max-height: 8rem;
    overflow-y: auto;
}
</style>
