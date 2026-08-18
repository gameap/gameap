<template>
    <div class="flex flex-col">
        <div class="text-sm text-muted mb-2">{{ selectedItem?.basename }}</div>
        <div class="flex text-center justify-center items-center min-h-[200px]">
            <n-spin v-if="!imgSrc" size="large" />
            <img
                v-else
                :src="imgSrc"
                :alt="selectedItem?.basename"
                :style="{ 'max-height': maxHeight + 'px' }"
                class="max-w-full"
            />
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../../stores/useSettingsStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'
import GET from '../../../http/get.js'

const fm = useFileManagerStore()
const settings = useSettingsStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const imgSrc = ref(null)

const auth = computed(() => settings.authHeader)
const selectedDisk = computed(() => fm.selectedDisk)
const selectedItem = computed(() => fm.selectedItems[0])

const maxHeight = computed(() => {
    return Math.min(window.innerHeight - 300, 600)
})

function loadImage() {
    if (auth.value) {
        GET.preview(selectedDisk.value, selectedItem.value.path).then((response) => {
            const mimeType = response.headers['content-type'].toLowerCase()
            const imgBase64 = btoa(String.fromCharCode.apply(null, new Uint8Array(response.data)))
            imgSrc.value = `data:${mimeType};base64,${imgBase64}`
        })
    } else {
        imgSrc.value = `${settings.baseUrl}preview?disk=${selectedDisk.value}&path=${encodeURIComponent(selectedItem.value.path)}&v=${selectedItem.value.timestamp}`
    }
}

onMounted(() => {
    loadImage()
})

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal }
    ]),
})
</script>
