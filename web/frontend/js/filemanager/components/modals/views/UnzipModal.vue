<template>
    <div>
        <div v-if="!archiveItem" class="text-red-500 dark:text-red-400">
            {{ lang.modal.unzip.noSelected }}
        </div>
        <div v-else>
            <div class="mb-3">
                <strong class="break-all">{{ archiveItem.basename }}</strong>
            </div>

            <div class="mb-2">
                <span class="block mb-1">{{ lang.modal.unzip.fieldRadioName }}</span>
                <n-radio-group v-model:value="target">
                    <n-radio value="current">{{ lang.modal.unzip.fieldRadio1 }}</n-radio>
                    <n-radio value="new">{{ lang.modal.unzip.fieldRadio2 }}</n-radio>
                </n-radio-group>
            </div>

            <div v-if="target === 'new'" class="mb-2">
                <label for="fm-unzip-folder" class="block mb-1">{{ lang.modal.unzip.fieldName }}</label>
                <n-input
                    id="fm-unzip-folder"
                    v-model:value="folderName"
                    @keyup.enter="submit"
                />
                <div v-if="folderExists" class="text-orange-500 dark:text-orange-400 text-sm mt-1">
                    {{ lang.modal.unzip.fieldFeedback }}
                </div>
            </div>

            <div class="mb-2">
                <span class="block mb-1">{{ lang.modal.unzip.conflictPolicy }}</span>
                <n-radio-group v-model:value="conflictPolicy">
                    <n-radio value="skip">{{ lang.modal.unzip.conflictSkip }}</n-radio>
                    <n-radio value="overwrite">{{ lang.modal.unzip.conflictOverwrite }}</n-radio>
                    <n-radio value="error">{{ lang.modal.unzip.conflictError }}</n-radio>
                </n-radio-group>
            </div>

            <div v-if="conflictPolicy === 'overwrite'" class="text-orange-500 dark:text-orange-400 text-sm">
                {{ lang.modal.unzip.warning }}
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, computed, inject } from 'vue'
import { NInput, NRadio, NRadioGroup } from 'naive-ui'
import POST from '../../../http/post.js'
import { joinPath } from '../../../http/upload-conflicts.js'
import { stripArchiveSuffix } from '../../../archive.js'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useArchiveOperationsStore } from '../../../stores/useArchiveOperationsStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const fm = useFileManagerStore()
const ops = useArchiveOperationsStore()
const { lang } = useTranslate()
const { hideModal } = useModal()
const archiveSocket = inject('fm-archive-socket', null)

const archiveItem = computed(() => {
    const item = fm.selectedItems[0]

    return item && item.type === 'file' ? item : null
})

function sanitizeName(value) {
    return String(value || '').replace(/[/\\]/g, '').trim()
}

const target = ref('new')
const folderName = ref(sanitizeName(stripArchiveSuffix(archiveItem.value?.basename || '')) || 'extracted')
const conflictPolicy = ref('skip')
const submitting = ref(false)

const folderExists = computed(
    () => target.value === 'new' && fm.directoryExist(fm.activeManager, sanitizeName(folderName.value)),
)

const submitDisabled = computed(
    () =>
        submitting.value ||
        !archiveItem.value ||
        (target.value === 'new' && sanitizeName(folderName.value) === ''),
)

async function submit() {
    if (submitDisabled.value) return

    submitting.value = true

    const currentDir = fm.selectedDirectory || ''
    const toNew = target.value === 'new'
    const destination = toNew ? joinPath(currentDir, sanitizeName(folderName.value)) : currentDir

    try {
        if (archiveSocket) {
            await archiveSocket.connectAndWait()
        }

        const response = await POST.extract({
            disk: fm.selectedDisk,
            path: archiveItem.value.path,
            destination,
            conflict_policy: conflictPolicy.value,
            create_destination: true,
        })

        ops.add({
            id: response.data.operation_id,
            type: 'extract',
            label: archiveItem.value.basename,
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
            icon: 'box-open',
            action: submit,
            disabled: submitDisabled.value,
        },
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>
