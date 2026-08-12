<template>
    <div>
        <div v-if="files.length === 0" class="text-red-500 dark:text-red-400">
            {{ lang.modal.hash.noSelected }}
        </div>
        <div v-else-if="phase !== 'results'">
            <div class="mb-3">
                <div v-if="files.length === 1">
                    <strong class="break-all">{{ files[0].basename }}</strong>
                    <span class="ml-2 text-stone-500">{{ bytesToHuman(files[0].size) }}</span>
                </div>
                <div v-else>
                    <ul class="fm-hash-file-list mb-1">
                        <li v-for="file in files" :key="file.path" class="text-sm truncate" :title="file.path">
                            {{ file.basename }}
                            <span class="ml-1 text-stone-500">{{ bytesToHuman(file.size) }}</span>
                        </li>
                    </ul>
                </div>
            </div>

            <div class="mb-2">
                <label for="fm-hash-algorithm" class="block mb-1">{{ lang.modal.hash.algorithm }}</label>
                <n-select
                    id="fm-hash-algorithm"
                    v-model:value="algorithm"
                    :options="algorithmOptions"
                    :disabled="phase === 'loading'"
                />
            </div>
        </div>
        <div v-else>
            <div class="mb-2 text-sm text-stone-500 dark:text-stone-400">
                {{ lang.modal.hash.copyHint }}
            </div>
            <table class="fm-hash-table w-full">
                <thead>
                    <tr>
                        <th class="text-left">{{ lang.modal.hash.file }}</th>
                        <th class="text-left">{{ lang.modal.hash.hash }} ({{ resultAlgorithm }})</th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="item in results" :key="item.path">
                        <td class="align-top">
                            <span class="break-all" :title="item.path">{{ basenameOf(item.path) }}</span>
                            <span v-if="item.size" class="block text-xs text-stone-500">
                                {{ bytesToHuman(item.size) }}
                            </span>
                        </td>
                        <td class="align-top">
                            <span v-if="item.error" class="text-red-500 dark:text-red-400 text-sm break-all">
                                {{ lang.response[item.error] ?? item.error }}
                            </span>
                            <span
                                v-else
                                class="fm-hash-value font-mono text-xs break-all cursor-pointer"
                                role="button"
                                tabindex="0"
                                :title="lang.modal.hash.copyHint"
                                @click="copyHash(item)"
                                @keydown.enter.prevent="copyHash(item)"
                                @keydown.space.prevent="copyHash(item)"
                            >
                                {{ item.hash }}
                                <span v-if="copiedPath === item.path" class="ml-1 text-green-600 dark:text-green-400">
                                    {{ lang.modal.hash.copied }}
                                </span>
                            </span>
                        </td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { NSelect } from 'naive-ui'
import { copyToClipboard } from '@/utils/clipboard.js'
import POST from '../../../http/post.js'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useHelper } from '../../../composables/useHelper.js'
import { useModal } from '../../../composables/useModal.js'

const ALGORITHMS = ['md5', 'sha1', 'sha256', 'sha512', 'crc32', 'crc64']

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { bytesToHuman } = useHelper()
const { hideModal } = useModal()

const phase = ref('form')
const algorithm = ref('sha256')
const results = ref([])
const resultAlgorithm = ref('')
const copiedPath = ref(null)

const files = computed(() => fm.selectedItems.filter((item) => item.type === 'file'))

const algorithmOptions = ALGORITHMS.map((value) => ({ value, label: value }))

function basenameOf(p) {
    return String(p || '').split('/').filter(Boolean).pop() || p
}

function compute() {
    if (files.value.length === 0 || phase.value === 'loading') return

    phase.value = 'loading'

    POST.hash({
        disk: fm.selectedDisk,
        paths: files.value.map((file) => file.path),
        algorithm: algorithm.value,
    })
        .then((response) => {
            results.value = response.data.items || []
            resultAlgorithm.value = response.data.algorithm || algorithm.value
            phase.value = 'results'
        })
        .catch(() => {
            // The global interceptor already surfaced the error.
            phase.value = 'form'
        })
}

let copiedTimer = null

function copyHash(item) {
    copyToClipboard(item.hash)
    copiedPath.value = item.path
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
        copiedPath.value = null
    }, 1500)
}

function back() {
    phase.value = 'form'
    results.value = []
    copiedPath.value = null
}

defineExpose({
    footerButtons: computed(() => {
        if (phase.value === 'results') {
            return [
                { label: lang.value.btn.back, color: 'black', icon: 'chevron-left', action: back },
                { label: lang.value.btn.close, color: 'black', icon: 'close', action: hideModal },
            ]
        }

        return [
            {
                label: lang.value.modal.hash.compute,
                color: 'green',
                icon: 'fingerprint',
                action: compute,
                disabled: files.value.length === 0 || phase.value === 'loading',
            },
            { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        ]
    }),
})
</script>

<style lang="scss">
.fm-hash-file-list {
    max-height: 10rem;
    overflow-y: auto;
}

.fm-hash-table {
    th,
    td {
        padding: 0.4rem 0.75rem;
    }

    thead th {
        font-weight: 600;
        @apply border-b;
    }

    tbody tr:hover {
        @apply bg-stone-50 dark:bg-stone-800;
    }
}

.fm-hash-value:hover,
.fm-hash-value:focus-visible {
    @apply text-sky-600 dark:text-sky-400;
}

.fm-hash-value:focus-visible {
    @apply outline outline-1 outline-sky-500 rounded-sm;
}
</style>
