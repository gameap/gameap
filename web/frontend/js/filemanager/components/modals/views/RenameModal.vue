<template>
    <div class="mb-3">
        <label for="fm-input-rename" class="block mb-2">{{ lang.modal.rename.fieldName }}</label>
        <n-input
            id="fm-input-rename"
            ref="renameInput"
            v-model:value="name"
            :status="checkName && name ? 'error' : undefined"
            @keyup="validateName"
            @keyup.enter="!submitDisable && rename()"
        />
        <div v-if="checkName && name" class="text-danger text-sm mt-1">
            {{ lang.modal.rename.fieldFeedback }}
            {{ directoryExist ? ` - ${lang.modal.rename.directoryExist}` : '' }}
            {{ fileExist ? ` - ${lang.modal.rename.fileExist}` : '' }}
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

const name = ref('')
const directoryExist = ref(false)
const fileExist = ref(false)
const renameInput = ref(null)

const selectedItem = computed(() => fm.getSelectedList(activeManager.value)[0])
const checkName = computed(() => directoryExist.value || fileExist.value || !name.value)
const submitDisable = computed(() => checkName.value || name.value === selectedItem.value?.basename)

onMounted(() => {
    name.value = selectedItem.value?.basename || ''
    renameInput.value?.focus()
})

function validateName() {
    if (name.value !== selectedItem.value?.basename) {
        if (selectedItem.value?.type === 'dir') {
            directoryExist.value = fm.directoryExist(activeManager.value, name.value)
        } else {
            fileExist.value = fm.fileExist(activeManager.value, name.value)
        }
    }
}

function rename() {
    const newName = selectedItem.value.dirname
        ? `${selectedItem.value.dirname}/${name.value}`
        : name.value

    fm.rename({
        type: selectedItem.value.type,
        newName,
        oldName: selectedItem.value.path,
    }).then(() => {
        hideModal()
    })
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        { label: lang.value.btn.submit, color: 'green', icon: 'edit', action: rename, disabled: submitDisable.value },
    ]),
})
</script>
