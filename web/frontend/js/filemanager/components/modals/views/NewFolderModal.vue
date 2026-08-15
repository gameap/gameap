<template>
    <div class="mb-3">
        <label for="fm-folder-name" class="block mb-2">{{ lang.modal.newFolder.fieldName }}</label>
        <n-input
            id="fm-folder-name"
            ref="folderNameInput"
            v-model:value="directoryName"
            :status="directoryExist ? 'error' : undefined"
            @keyup="validateDirName"
            @keyup.enter="submitActive && addFolder()"
        />
        <div v-if="directoryExist" class="text-danger text-sm mt-1">
            {{ lang.modal.newFolder.fieldFeedback }}
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { activeManager, hideModal } = useModal()

const directoryName = ref('')
const directoryExist = ref(false)
const folderNameInput = ref(null)

const submitActive = computed(() => directoryName.value && !directoryExist.value)

onMounted(() => {
    folderNameInput.value?.focus()
})

function validateDirName() {
    if (directoryName.value) {
        directoryExist.value = fm.directoryExist(activeManager.value, directoryName.value)
    } else {
        directoryExist.value = false
    }
}

function addFolder() {
    fm.createDirectory(directoryName.value).then((response) => {
        if (response.data.result.status === 'success') {
            hideModal()
        }
    })
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        { label: lang.value.btn.submit, color: 'green', icon: 'add-folder', action: addFolder, disabled: !submitActive.value },
    ]),
})
</script>
