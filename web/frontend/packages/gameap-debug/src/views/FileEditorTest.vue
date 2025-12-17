<template>
    <div class="h-full flex">
        <!-- File Browser Panel -->
        <div class="w-72 border-r border-stone-200 dark:border-stone-700">
            <FileBrowser @select="selectFile" />
        </div>

        <!-- Editor Panel -->
        <div class="flex-1 flex flex-col">
            <!-- Editor Header -->
            <div
                v-if="selectedFile"
                class="p-3 border-b border-stone-200 dark:border-stone-700 bg-stone-50 dark:bg-stone-900 flex items-center justify-between"
            >
                <div>
                    <h3 class="font-semibold text-stone-800 dark:text-stone-200">
                        {{ selectedFile.basename }}
                    </h3>
                    <p class="text-sm text-stone-500 dark:text-stone-400">
                        {{ selectedFile.path }}
                    </p>
                </div>
                <div class="flex items-center gap-2">
                    <button
                        v-if="editorRef"
                        @click="triggerSave"
                        class="px-3 py-1.5 bg-green-600 hover:bg-green-700 text-white rounded-md text-sm flex items-center"
                    >
                        <i class="fa-solid fa-floppy-disk mr-2"></i>
                        Save
                    </button>
                    <button
                        @click="closeEditor"
                        class="px-3 py-1.5 bg-stone-200 dark:bg-stone-700 hover:bg-stone-300 dark:hover:bg-stone-600 text-stone-700 dark:text-stone-300 rounded-md text-sm flex items-center"
                    >
                        <i class="fa-solid fa-xmark mr-2"></i>
                        Close
                    </button>
                </div>
            </div>

            <!-- Matching Editors Info -->
            <div
                v-if="selectedFile && matchingEditors.length > 0"
                class="px-3 py-2 bg-blue-50 dark:bg-blue-900/20 border-b border-blue-100 dark:border-blue-900"
            >
                <span class="text-sm text-blue-700 dark:text-blue-300">
                    <i class="fa-solid fa-info-circle mr-2"></i>
                    {{ matchingEditors.length }} matching editor(s):
                    <span
                        v-for="(editor, idx) in matchingEditors"
                        :key="idx"
                        class="ml-2"
                    >
                        <span
                            :class="editor.isDefault ? 'font-semibold' : ''"
                        >
                            {{ editor.editor.name }}
                        </span>
                        <span class="text-xs opacity-75">(score: {{ editor.score }})</span>
                        <span v-if="idx < matchingEditors.length - 1">,</span>
                    </span>
                </span>
            </div>

            <!-- Editor Content -->
            <div v-if="selectedFile && matchingEditor" class="flex-1 overflow-auto">
                <component
                    ref="editorRef"
                    :is="matchingEditor.editor.component"
                    :content="fileContent"
                    :file-path="selectedFile.path"
                    :file-name="selectedFile.basename"
                    :extension="selectedFile.extension"
                    :game-code="debugStore.currentServer?.game_id"
                    :game-name="debugStore.currentServer?.name"
                    :plugin-id="matchingEditor.pluginId"
                    @save="handleSave"
                    @close="handleClose"
                />
            </div>

            <!-- No File Selected -->
            <div
                v-else-if="!selectedFile"
                class="flex-1 flex items-center justify-center text-stone-500 dark:text-stone-400"
            >
                <div class="text-center">
                    <i class="fa-solid fa-file-lines text-4xl mb-4 opacity-50"></i>
                    <p>{{ t('debug.select_file') }}</p>
                </div>
            </div>

            <!-- No Matching Editor -->
            <div
                v-else-if="selectedFile && !matchingEditor"
                class="flex-1 flex items-center justify-center text-stone-500 dark:text-stone-400"
            >
                <div class="text-center">
                    <i class="fa-solid fa-puzzle-piece text-4xl mb-4 opacity-50"></i>
                    <p>No matching editor for this file type</p>
                    <p class="text-sm mt-2">
                        Extension: <code class="bg-stone-200 dark:bg-stone-700 px-2 py-0.5 rounded">.{{ selectedFile.extension }}</code>
                    </p>
                </div>
            </div>

            <!-- Event Log -->
            <EventLog :events="debugStore.eventLog" class="h-48" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, computed, markRaw } from 'vue'
import FileBrowser from '@/components/FileBrowser.vue'
import EventLog from '@/components/EventLog.vue'
import { useDebugStore } from '@/stores/debug'
import { usePluginsStore } from '@/stores/plugins'
import type { MockFile } from '@/mocks/files'

const debugStore = useDebugStore()
const pluginsStore = usePluginsStore()

const selectedFile = ref<MockFile | null>(null)
const editorRef = ref<{ save?: () => void; close?: () => void } | null>(null)

const fileContent = computed(() => {
    if (!selectedFile.value) return null
    return selectedFile.value.content
})

const matchingEditors = computed(() => {
    if (!selectedFile.value) return []

    return pluginsStore.getMatchingEditors(
        {
            fileName: selectedFile.value.basename,
            filePath: selectedFile.value.path,
            extension: selectedFile.value.extension,
        },
        {
            gameCode: debugStore.currentServer?.game_id,
            gameName: debugStore.currentServer?.name,
        }
    )
})

const matchingEditor = computed(() => {
    const editors = matchingEditors.value
    if (editors.length === 0) return null

    // Return the default (highest score) editor with component marked as raw
    const editor = editors[0]
    return {
        ...editor,
        editor: {
            ...editor.editor,
            component: markRaw(editor.editor.component),
        },
    }
})

function selectFile(file: MockFile) {
    selectedFile.value = file
    debugStore.logEvent('file-selected', { path: file.path, type: file.type })
}

function handleSave(content: string | ArrayBuffer) {
    debugStore.logEvent('save', content)
    console.log('[Debug] Save event received:', content)
}

function handleClose() {
    debugStore.logEvent('close', null)
    console.log('[Debug] Close event received')
}

function triggerSave() {
    if (editorRef.value?.save) {
        editorRef.value.save()
    }
}

function closeEditor() {
    if (editorRef.value?.close) {
        editorRef.value.close()
    }
    selectedFile.value = null
}

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
