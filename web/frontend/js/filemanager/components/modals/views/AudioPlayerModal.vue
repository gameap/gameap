<template>
    <div class="fm-modal-audio-player">
        <audio ref="fmAudio" controls class="w-full" />
        <GDivider />
        <div
            class="flex justify-between items-center py-2 px-2 rounded cursor-pointer"
            :class="playingIndex === index ? 'bg-surface-hover' : 'hover:bg-surface-hover'"
            v-for="(item, index) in audioFiles"
            :key="index"
        >
            <div class="truncate flex-1">
                <span class="text-faint mr-2">{{ index }}.</span>
                {{ item.basename }}
            </div>
            <template v-if="playingIndex === index">
                <n-button quaternary circle @click="togglePlay()">
                    <template #icon>
                        <GIcon v-if="status === 'playing'" name="close" />
                        <GIcon v-else name="play" class="text-info" />
                    </template>
                </n-button>
            </template>
            <template v-else>
                <n-button quaternary circle @click="selectTrack(index)">
                    <template #icon>
                        <GIcon name="play" />
                    </template>
                </n-button>
            </template>
        </div>
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { GIcon, GDivider } from '@gameap/ui'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../../stores/useSettingsStore.js'

const fm = useFileManagerStore()
const settings = useSettingsStore()

const fmAudio = ref(null)
const playingIndex = ref(0)
const status = ref('paused')

const selectedDisk = computed(() => fm.selectedDisk)
const audioFiles = computed(() => fm.selectedItems)

function getSourceUrl(index) {
    return `${settings.baseUrl}/stream-file?disk=${selectedDisk.value}&path=${encodeURIComponent(audioFiles.value[index].path)}`
}

function setSource(index) {
    fmAudio.value.src = getSourceUrl(index)
}

function selectTrack(index) {
    if (!fmAudio.value.paused) {
        fmAudio.value.pause()
        fmAudio.value.currentTime = 0
    }
    setSource(index)
    fmAudio.value.play()
    playingIndex.value = index
}

function togglePlay() {
    if (fmAudio.value.paused) {
        fmAudio.value.play()
    } else {
        fmAudio.value.pause()
    }
}

onMounted(() => {
    setSource(playingIndex.value)

    fmAudio.value.addEventListener('play', () => {
        status.value = 'playing'
    })

    fmAudio.value.addEventListener('pause', () => {
        status.value = 'paused'
    })

    fmAudio.value.addEventListener('ended', () => {
        if (audioFiles.value.length > playingIndex.value + 1) {
            selectTrack(playingIndex.value + 1)
        }
    })
})

defineExpose({
    footerButtons: computed(() => []),
})
</script>
