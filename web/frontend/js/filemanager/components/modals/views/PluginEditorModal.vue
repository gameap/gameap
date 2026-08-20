<template>
    <div class="flex flex-col">
        <div v-if="contentLoaded" class="plugin-editor-container">
            <component
                ref="editorRef"
                :is="editorComponent"
                :content="content"
                :file-path="filePath"
                :file-name="fileName"
                :extension="extension"
                :game-code="gameCode"
                :game-name="gameName"
                :file-size="fileSize"
                :file-mtime="fileMtime"
                :disk="selectedDisk"
                :plugin-id="pluginId"
                @save="handleSave"
                @close="handleClose"
            />
        </div>
        <div class="flex justify-center items-center" v-else :style="{ height: '300px' }">
            <n-spin size="large" />
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted, markRaw } from 'vue'
import { useFileManagerStore } from '@/filemanager/stores'
import { useModalStore } from '@/filemanager/stores'
import { useTranslate, useFileEditors } from '@/filemanager/composables'

const fm = useFileManagerStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { gameCode, gameName } = useFileEditors()

const content = ref(null)
const contentLoaded = ref(false)
const editorRef = ref(null)

const editorState = computed(() => modal.pluginEditorState)
const pluginId = computed(() => editorState.value?.pluginId)
const editor = computed(() => editorState.value?.editor)
const file = computed(() => editorState.value?.file)

const fileName = computed(() => file.value?.basename || '')
const filePath = computed(() => file.value?.path || '')
const extension = computed(() => file.value?.extension || '')
const fileSize = computed(() => file.value?.size)
const fileMtime = computed(() => file.value?.timestamp)
const editorComponent = computed(() => editor.value?.component ? markRaw(editor.value.component) : null)
const isReadOnly = computed(() => editor.value?.readOnly || false)
const contentType = computed(() => editor.value?.contentType || 'text')

const selectedDisk = computed(() => fm.selectedDisk)

function handleSave(newContent) {
    const formData = new FormData()
    formData.append('disk', selectedDisk.value)
    formData.append('path', file.value.dirname)

    const blob = contentType.value === 'binary'
        ? new Blob([newContent])
        : new Blob([newContent], { type: 'text/plain' })

    formData.append('file', blob, file.value.basename)

    fm.updateFile(formData).then((response) => {
        if (response.data.result.status === 'success') {
            modal.clearModal()
        }
    })
}

function triggerSave() {
    if (editorRef.value?.save) {
        editorRef.value.save()
    }
}

function handleClose() {
    modal.clearModal()
}

onMounted(() => {
    if (!file.value || !editor.value) {
        modal.clearModal()
        return
    }

    // An editor that declares 'none' is handed no content: it decides what to
    // read, which is the only way to work on a file past the edit size cap.
    if (contentType.value === 'none') {
        contentLoaded.value = true
        return
    }

    if (contentType.value === 'binary') {
        fm.getFileArrayBuffer({
            disk: selectedDisk.value,
            path: file.value.path,
        })
            .then((response) => {
                content.value = response.data
                contentLoaded.value = true
            })
            .catch(() => {
                modal.clearModal()
            })
    } else {
        fm.getFile({
            disk: selectedDisk.value,
            path: file.value.path,
        })
            .then((response) => {
                content.value = response.data
                contentLoaded.value = true
            })
            .catch(() => {
                modal.clearModal()
            })
    }
})

defineExpose({
    footerButtons: computed(() => {
        const buttons = [{
            label: lang.value.btn.cancel,
            color: 'black',
            icon: 'close',
            action: handleClose
        }]

        if (!isReadOnly.value) {
            buttons.push({
                label: lang.value.btn.submit,
                color: 'green',
                icon: 'save',
                action: triggerSave
            })
        }

        return buttons
    }),
})
</script>

<style scoped>
.plugin-editor-container {
    max-height: calc(100vh - 250px);
    overflow: auto;
}
</style>
