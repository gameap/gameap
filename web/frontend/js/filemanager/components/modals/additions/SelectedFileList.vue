<template>
    <div class="fm-additions-file-list">
        <div class="flex items-center justify-between gap-3" v-for="(item, index) in selectedItems" v-bind:key="index">
            <div class="min-w-0 truncate">
                <span v-if="item.type === 'dir'"> <GIcon name="folder" />{{ item.basename }} </span>
                <span v-else>
                    <GIcon :name="extensionToIcon(item.extension)" /> {{ item.basename }}
                </span>
            </div>
            <div class="shrink-0 text-muted" v-if="item.type === 'file'">
                {{ bytesToHuman(item.size) }}
            </div>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useHelper } from '../../../composables/useHelper.js'

const fm = useFileManagerStore()
const { bytesToHuman, extensionToIcon } = useHelper()

const selectedItems = computed(() => fm.selectedItems)
</script>

<style lang="scss">
.fm-additions-file-list {
    .g-icon {
        padding-right: 0.5rem;
    }
}
</style>
