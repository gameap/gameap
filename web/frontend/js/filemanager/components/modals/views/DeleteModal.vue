<template>
    <div>
        <div v-if="selectedItems.length">
            <selected-file-list />
        </div>
        <div v-else>
            <span class="text-danger">{{ lang.modal.delete.noSelected }}</span>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import SelectedFileList from '../additions/SelectedFileList.vue'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const selectedItems = computed(() => fm.selectedItems)

function deleteItems() {
    const items = selectedItems.value.map((item) => ({
        path: item.path,
        type: item.type,
    }))

    fm.delete(items).then(() => {
        hideModal()
    })
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        { label: lang.value.modal.delete.title, color: 'red', icon: 'delete', action: deleteItems, disabled: !selectedItems.value.length },
    ]),
})
</script>
