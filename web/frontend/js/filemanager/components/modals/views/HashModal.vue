<template>
    <div>
        <div v-if="!file" class="text-red-500 dark:text-red-400">
            {{ lang.modal.hash.noSelected }}
        </div>
        <div v-else>
            <div class="mb-3">
                <strong class="break-all">{{ file.basename }}</strong>
                <span v-if="file.size" class="ml-2 text-sm text-stone-500 dark:text-stone-400">
                    {{ bytesToHuman(file.size) }}
                </span>
            </div>

            <div
                v-for="algo in ROW_ORDER"
                :key="algo"
                class="fm-hash-row grid grid-cols-[6rem_minmax(0,1fr)_auto] gap-3 items-center p-1 rounded hover:bg-stone-100 dark:hover:bg-stone-800"
            >
                <strong class="text-sm">{{ algo.toUpperCase() }}</strong>

                <div class="min-w-0 flex items-center">
                    <GButton
                        v-if="cells[algo].status === 'idle'"
                        color="white"
                        size="small"
                        @click="computeAlgo(algo)"
                    >
                        <GIcon name="fingerprint" class="mr-1" />
                        {{ lang.modal.hash.compute }}
                    </GButton>

                    <GIcon
                        v-else-if="cells[algo].status === 'loading'"
                        name="spinner"
                        class="text-stone-500 dark:text-stone-400"
                    />

                    <template v-else-if="cells[algo].status === 'done'">
                        <span
                            :ref="(el) => setValueEl(algo, el)"
                            class="fm-hash-value min-w-0 font-mono text-xs truncate cursor-pointer"
                            role="button"
                            tabindex="0"
                            :title="cells[algo].value"
                            @click="copyValue(algo)"
                            @keydown.enter.prevent="copyValue(algo)"
                            @keydown.space.prevent="copyValue(algo)"
                        >
                            {{ cells[algo].value }}
                        </span>
                        <span
                            v-if="copiedAlgo === algo"
                            class="ml-2 shrink-0 text-xs text-green-600 dark:text-green-400"
                        >
                            {{ lang.modal.hash.copied }}
                        </span>
                        <span
                            v-else-if="manualAlgo === algo"
                            class="ml-2 shrink-0 text-xs text-amber-600 dark:text-amber-400"
                        >
                            {{ lang.modal.hash.copyManual }}
                        </span>
                    </template>

                    <template v-else>
                        <span class="min-w-0 text-sm break-all text-red-500 dark:text-red-400">
                            {{ errorText(algo) }}
                        </span>
                        <GButton
                            color="white"
                            size="small"
                            class="ml-2 shrink-0"
                            @click="computeAlgo(algo)"
                        >
                            {{ lang.modal.hash.compute }}
                        </GButton>
                    </template>
                </div>

                <div class="w-6 text-right cursor-pointer">
                    <GIcon
                        v-if="cells[algo].status === 'done'"
                        name="copy"
                        :title="lang.modal.hash.copyHint"
                        @click="copyValue(algo)"
                    />
                </div>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { GIcon } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { copyToClipboard } from '@/utils/clipboard.js'
import POST from '../../../http/post.js'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useHelper } from '../../../composables/useHelper.js'
import { useModal } from '../../../composables/useModal.js'

const ROW_ORDER = ['sha256', 'sha1', 'sha512', 'md5', 'crc32', 'crc64']
const SMALL_FILE_BYTES = 1024 * 1024
const AUTO_ALGOS_SMALL = ['sha256', 'md5']
const AUTO_ALGOS_LARGE = ['sha256']

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { bytesToHuman } = useHelper()
const { hideModal } = useModal()

// The selection is snapshotted at mount so listing refreshes cannot swap the
// file under an open modal; the view unmounts on close, resetting everything.
const selected = fm.selectedItems.find((item) => item.type === 'file')
const file = selected
    ? { path: selected.path, basename: selected.basename, size: Number(selected.size) || 0 }
    : null
const disk = fm.selectedDisk

const cells = reactive(
    Object.fromEntries(ROW_ORDER.map((algo) => [algo, { status: 'idle', value: '', error: '' }])),
)
const copiedAlgo = ref(null)
const manualAlgo = ref(null)
const abort = new AbortController()
let copiedTimer = null

const valueEls = {}

function setValueEl(algo, el) {
    valueEls[algo] = el
}

function computeAlgo(algo) {
    const cell = cells[algo]
    if (!file || cell.status === 'loading' || cell.status === 'done') return

    cell.status = 'loading'
    cell.value = ''
    cell.error = ''

    POST.hash({ disk, paths: [file.path], algorithm: algo }, { signal: abort.signal })
        .then((response) => {
            const item = (response.data.items || []).find((it) => it.path === file.path)
            if (!item) {
                cell.status = 'error'
                cell.error = 'notFound'
            } else if (item.error) {
                cell.status = 'error'
                cell.error = item.error
            } else {
                cell.status = 'done'
                cell.value = item.hash
            }
        })
        .catch(() => {
            // The global interceptor already surfaced the error.
            if (abort.signal.aborted) return

            cell.status = 'error'
            cell.error = ''
        })
}

function errorText(algo) {
    const error = cells[algo].error
    if (!error) return lang.value.modal.hash.failed

    return lang.value.response[error] ?? error
}

async function copyValue(algo) {
    if (cells[algo].status !== 'done') return

    if (await copyToClipboard(cells[algo].value)) {
        manualAlgo.value = null
        copiedAlgo.value = algo
        clearTimeout(copiedTimer)
        copiedTimer = setTimeout(() => {
            copiedAlgo.value = null
        }, 1500)

        return
    }

    // No clipboard access (e.g. blocked execCommand on a plain-http origin):
    // select the value and let the user press Ctrl+C themselves.
    selectValue(algo)
    copiedAlgo.value = null
    manualAlgo.value = algo
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
        manualAlgo.value = null
    }, 3000)
}

function selectValue(algo) {
    const el = valueEls[algo]
    const selection = window.getSelection()
    if (!el || !selection) return

    const range = document.createRange()
    range.selectNodeContents(el)
    selection.removeAllRanges()
    selection.addRange(range)
}

onMounted(() => {
    if (!file) return

    const autoAlgos = file.size <= SMALL_FILE_BYTES ? AUTO_ALGOS_SMALL : AUTO_ALGOS_LARGE
    autoAlgos.forEach(computeAlgo)
})

onUnmounted(() => {
    abort.abort()
    clearTimeout(copiedTimer)
})

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.close, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>

<style lang="scss">
.fm-hash-value:hover,
.fm-hash-value:focus-visible {
    @apply text-sky-600 dark:text-sky-400;
}

.fm-hash-value:focus-visible {
    @apply outline outline-1 outline-sky-500 rounded-sm;
}
</style>
