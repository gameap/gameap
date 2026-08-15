<template>
    <div class="mb-3">
        <label for="fm-file-name" class="block mb-2">{{ lang.modal.newFile.fieldName }}</label>
        <n-input
            id="fm-file-name"
            ref="fileNameInput"
            v-model:value="fileName"
            :status="fileExist ? 'error' : undefined"
            @keyup="validateFileName"
            @keyup.enter="submitActive && addFile()"
        />
        <div v-if="fileExist" class="text-danger text-sm mt-1">
            {{ lang.modal.newFile.fieldFeedback }}
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

const fileName = ref('')
const fileExist = ref(false)
const fileNameInput = ref(null)

const submitActive = computed(() => fileName.value && !fileExist.value)

onMounted(() => {
    fileNameInput.value?.focus()
})

function validateFileName() {
    if (fileName.value) {
        fileExist.value = fm.fileExist(activeManager.value, fileName.value)
    } else {
        fileExist.value = false
    }
}

function addFile() {
    fm.createFile(fileName.value).then((response) => {
        if (response.data.result.status === 'success') {
            hideModal()
        }
    })
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        { label: lang.value.btn.submit, color: 'green', icon: 'file', action: addFile, disabled: !submitActive.value },
    ]),
})
</script>
