<template>
    <div class="flex flex-col">
        <div class="text-sm text-stone-500 mb-2">{{ selectedItem?.basename }}</div>
        <div v-if="codeLoaded">
            <codemirror
                ref="fmCodeEditor"
                v-model="code"
                :style="{ height: editorHeight + 'px' }"
                :extensions="extensions"
                @change="onChange"
            />
        </div>
        <div class="flex justify-center items-center" v-else :style="{ height: editorHeight + 'px' }">
            <n-spin size="large" />
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Codemirror } from 'vue-codemirror'
import { javascript } from '@codemirror/lang-javascript'
import { xml } from '@codemirror/lang-xml'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'

import { useFileManagerStore } from '@/filemanager/stores'
import { useTranslate } from '@/filemanager/composables'
import { useModal } from '@/filemanager/composables'

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const code = ref('')
const extensions = [javascript(), xml(), json(), oneDark]
const editedCode = ref('')
const codeLoaded = ref(false)
const fmCodeEditor = ref(null)

const selectedDisk = computed(() => fm.selectedDisk)
const selectedItem = computed(() => fm.selectedItems[0])

const editorHeight = computed(() => {
    return Math.min(window.innerHeight - 300, 500)
})

function onChange(value) {
    editedCode.value = value
}

function updateFile() {
    const formData = new FormData()
    formData.append('disk', selectedDisk.value)
    formData.append('path', selectedItem.value.dirname)
    formData.append('file', new Blob([editedCode.value]), selectedItem.value.basename)

    fm.updateFile(formData).then((response) => {
        if (response.data.result.status === 'success') {
            hideModal()
        }
    })
}

onMounted(() => {
    fm.getFile({
        disk: selectedDisk.value,
        path: selectedItem.value.path,
    })
        .then((response) => {
            code.value = response.data
            editedCode.value = response.data
            codeLoaded.value = true
        })
        .catch(() => {
            hideModal()
        })
})

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.submit, color: 'green', icon: 'fa-solid fa-floppy-disk', action: updateFile },
        { label: lang.value.btn.cancel, color: 'black', icon: 'fa-solid fa-xmark', action: hideModal },
    ]),
})
</script>
