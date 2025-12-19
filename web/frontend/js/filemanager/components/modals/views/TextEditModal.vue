<template>
    <div class="flex flex-col">
        <div class="text-sm text-stone-500 mb-2">{{ selectedItem?.basename }}</div>
        <div v-if="codeLoaded" class="code-editor" :style="{ height: editorHeight + 'px' }">
            <div class="line-numbers" ref="lineNumbersRef">
                <span v-for="n in lineCount" :key="n">{{ n }}</span>
            </div>
            <textarea
                ref="textareaRef"
                v-model="code"
                spellcheck="false"
                @scroll="syncScroll"
                @keydown.tab.prevent="insertTab"
            />
        </div>
        <div class="flex justify-center items-center" v-else :style="{ height: editorHeight + 'px' }">
            <n-spin size="large" />
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

import { useFileManagerStore } from '@/filemanager/stores'
import { useTranslate } from '@/filemanager/composables'
import { useModal } from '@/filemanager/composables'

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const code = ref('')
const codeLoaded = ref(false)
const textareaRef = ref(null)
const lineNumbersRef = ref(null)

const selectedDisk = computed(() => fm.selectedDisk)
const selectedItem = computed(() => fm.selectedItems[0])

const editorHeight = computed(() => {
    return Math.min(window.innerHeight - 300, 500)
})

const lineCount = computed(() => {
    return code.value.split('\n').length
})

function syncScroll() {
    if (lineNumbersRef.value && textareaRef.value) {
        lineNumbersRef.value.scrollTop = textareaRef.value.scrollTop
    }
}

function insertTab(e) {
    const textarea = e.target
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    code.value = code.value.substring(0, start) + '\t' + code.value.substring(end)
    nextTick(() => {
        textarea.selectionStart = textarea.selectionEnd = start + 1
    })
}

function nextTick(fn) {
    setTimeout(fn, 0)
}

function updateFile() {
    const formData = new FormData()
    formData.append('disk', selectedDisk.value)
    formData.append('path', selectedItem.value.dirname)
    formData.append('file', new Blob([code.value]), selectedItem.value.basename)

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

<style scoped>
.code-editor {
    display: flex;
    background: #1e1e1e;
    border-radius: 6px;
    overflow: hidden;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 14px;
    line-height: 1.5;
}

.line-numbers {
    background: #252526;
    color: #858585;
    padding: 10px 0;
    text-align: right;
    user-select: none;
    overflow: hidden;
    min-width: 50px;
}

.line-numbers span {
    display: block;
    padding: 0 12px 0 8px;
}

textarea {
    flex: 1;
    background: #1e1e1e;
    color: #d4d4d4;
    border: none;
    outline: none;
    resize: none;
    padding: 10px;
    font-family: inherit;
    font-size: inherit;
    line-height: inherit;
    tab-size: 4;
    white-space: pre;
    overflow-wrap: normal;
    overflow-x: auto;
}

textarea::selection {
    background: #264f78;
}
</style>
