<template>
    <div class="h-full flex flex-col bg-white dark:bg-stone-800">
        <div class="p-3 border-b border-stone-200 dark:border-stone-700">
            <h3 class="font-semibold text-stone-800 dark:text-stone-200">
                <i class="fa-solid fa-folder-open mr-2 text-yellow-500"></i>
                Test Files
            </h3>
        </div>

        <div class="flex-1 overflow-y-auto p-2">
            <ul class="space-y-1">
                <li
                    v-for="file in mockFiles"
                    :key="file.path"
                    @click="selectFile(file)"
                    class="flex items-center px-3 py-2 rounded-md cursor-pointer transition-colors"
                    :class="[
                        selectedPath === file.path
                            ? 'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300'
                            : 'hover:bg-stone-100 dark:hover:bg-stone-700 text-stone-700 dark:text-stone-300'
                    ]"
                >
                    <i :class="getFileIcon(file)" class="w-5 mr-3 text-stone-400"></i>
                    <div class="flex-1 min-w-0">
                        <div class="font-medium truncate">{{ file.basename }}</div>
                        <div class="text-xs text-stone-500 dark:text-stone-400 truncate">
                            {{ file.path }}
                        </div>
                    </div>
                    <div class="ml-2 flex items-center gap-2">
                        <span
                            :class="file.type === 'binary' ? 'bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300' : 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'"
                            class="text-xs px-2 py-0.5 rounded"
                        >
                            {{ file.type }}
                        </span>
                        <span class="text-xs text-stone-400">
                            {{ formatSize(file.size) }}
                        </span>
                    </div>
                </li>
            </ul>
        </div>

        <!-- File Info Panel -->
        <div v-if="selectedFile" class="p-3 border-t border-stone-200 dark:border-stone-700 bg-stone-50 dark:bg-stone-900">
            <div class="text-sm space-y-1 text-stone-600 dark:text-stone-400">
                <div class="flex justify-between">
                    <span>Name:</span>
                    <span class="font-medium text-stone-800 dark:text-stone-200">{{ selectedFile.basename }}</span>
                </div>
                <div class="flex justify-between">
                    <span>Extension:</span>
                    <span class="font-mono">.{{ selectedFile.extension }}</span>
                </div>
                <div class="flex justify-between">
                    <span>Type:</span>
                    <span>{{ selectedFile.type }}</span>
                </div>
                <div class="flex justify-between">
                    <span>Size:</span>
                    <span>{{ formatSize(selectedFile.size) }}</span>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { mockFiles, getFileIcon as getFileIconFn, type MockFile } from '@/mocks/files'

const emit = defineEmits<{
    select: [file: MockFile]
}>()

const selectedPath = ref<string | null>(null)

const selectedFile = computed(() => {
    if (!selectedPath.value) return null
    return mockFiles.find(f => f.path === selectedPath.value) || null
})

function selectFile(file: MockFile) {
    selectedPath.value = file.path
    emit('select', file)
}

function getFileIcon(file: MockFile): string {
    return getFileIconFn(file)
}

function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
</script>
