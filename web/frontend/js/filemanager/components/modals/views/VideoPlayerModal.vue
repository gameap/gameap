<template>
    <div class="fm-modal-video-player">
        <div class="text-sm text-stone-500 mb-2">{{ videoFile?.basename }}</div>
        <video controls :src="videoSrc" class="w-full max-h-[70vh]" />
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../../stores/useSettingsStore.js'

const fm = useFileManagerStore()
const settings = useSettingsStore()

const selectedDisk = computed(() => fm.selectedDisk)
const videoFile = computed(() => fm.selectedItems[0])
const videoSrc = computed(() =>
    `${settings.baseUrl}/stream-file?disk=${selectedDisk.value}&path=${encodeURIComponent(videoFile.value.path)}`
)

defineExpose({
    footerButtons: computed(() => []),
})
</script>
