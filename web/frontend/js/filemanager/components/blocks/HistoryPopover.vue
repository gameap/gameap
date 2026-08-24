<template>
    <n-popover
        v-if="history.hasAnyEntries"
        v-model:show="show"
        trigger="click"
        placement="bottom-end"
        raw
        v-bind:show-arrow="false"
    >
        <template #trigger>
            <button
                type="button"
                class="fm-history-btn"
                v-bind:class="{ 'fm-history-btn--open': show }"
                v-bind:title="lang.history.title"
                v-bind:aria-label="lang.history.title"
            >
                <GIcon name="history" />
            </button>
        </template>

        <div class="fm-history-pop">
            <div v-if="history.hasRecent" class="fm-history-tabs" role="tablist">
                <button
                    v-for="tab in tabs"
                    v-bind:key="tab.view"
                    type="button"
                    role="tab"
                    class="fm-history-tab"
                    v-bind:class="{ 'fm-history-tab--active': history.effectiveView === tab.view }"
                    v-bind:aria-selected="history.effectiveView === tab.view"
                    v-on:click="history.setView(tab.view)"
                >
                    {{ tab.label }}
                </button>
            </div>

            <template v-if="hasItems">
                <div v-if="history.dirsTop.length" class="fm-history-section">
                    <div class="fm-history-label">{{ lang.history.directories }}</div>
                    <button
                        v-for="item in history.dirsTop"
                        v-bind:key="`dir-${item.path}`"
                        type="button"
                        class="fm-history-item"
                        v-bind:title="item.path"
                        v-on:click="activateDir(item)"
                    >
                        <GIcon name="folder" class="fm-history-icon" />
                        <span class="fm-history-text">
                            <span class="fm-history-name">{{ item.basename }}</span>
                            <span class="fm-history-path">{{ parentLabel(item) }}</span>
                        </span>
                        <span v-if="showTime" class="fm-history-meta">{{ relTime(item) }}</span>
                    </button>
                </div>

                <div v-if="history.filesTop.length" class="fm-history-section">
                    <div class="fm-history-label">{{ lang.history.files }}</div>
                    <button
                        v-for="item in history.filesTop"
                        v-bind:key="`file-${item.path}`"
                        type="button"
                        class="fm-history-item"
                        v-bind:title="item.path"
                        v-on:click="activateFile(item)"
                    >
                        <GIcon v-bind:name="fileIcon(item)" class="fm-history-icon" />
                        <span class="fm-history-text">
                            <span class="fm-history-name">{{ item.basename }}</span>
                            <span class="fm-history-path">{{ parentLabel(item) }}</span>
                        </span>
                        <span v-if="showTime" class="fm-history-meta">{{ relTime(item) }}</span>
                    </button>
                </div>

            </template>

            <div v-else class="fm-history-empty">
                {{ lang.history.empty }}
                <small>{{ lang.history.emptyHint }}</small>
            </div>
        </div>
    </n-popover>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { GIcon } from '@gameap/ui'
import { notification } from '@/parts/dialogs.js'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useSettingsStore } from '../../stores/useSettingsStore.js'
import { useHistoryStore } from '../../stores/useHistoryStore.js'
import { useTranslate } from '../../composables/useTranslate.js'
import { useHelper } from '../../composables/useHelper.js'
import { useFileOpener } from '../../composables/useFileOpener.js'
import { formatRelativeTime } from '../../history.js'

const props = defineProps({
    manager: { type: String, default: 'left' },
})

const fm = useFileManagerStore()
const settings = useSettingsStore()
const history = useHistoryStore()
const { lang } = useTranslate()
const { extensionToIcon } = useHelper()
const { openFile } = useFileOpener(props.manager)

const show = ref(false)

const tabs = computed(() => [
    { view: 'recent', label: lang.value.history.tabRecent },
    { view: 'frequent', label: lang.value.history.tabFrequent },
])

const hasItems = computed(() => history.dirsTop.length > 0 || history.filesTop.length > 0)
const showTime = computed(() => history.effectiveView === 'recent')

watch(show, (open) => {
    if (open) {
        history.ensureKey(fm.getManager(props.manager).selectedDisk)
        history.touchNow()
        document.addEventListener('keydown', onKeydown, true)
    } else {
        document.removeEventListener('keydown', onKeydown, true)
    }
})

onBeforeUnmount(() => {
    document.removeEventListener('keydown', onKeydown, true)
})

// Capture phase: Escape must close the popover without reaching the file
// manager's global handler, which would clear the table selection.
function onKeydown(event) {
    if (event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    show.value = false
}

function parentLabel(item) {
    return item.dirname === '' ? '/' : item.dirname
}

function fileIcon(item) {
    const dot = item.basename.lastIndexOf('.')
    return extensionToIcon(dot > 0 ? item.basename.slice(dot + 1) : '')
}

function relTime(item) {
    return formatRelativeTime(
        item.lastAt,
        history.nowTick,
        lang.value.history.time,
        (ms) => new Date(ms).toLocaleDateString(settings.lang || undefined)
    )
}

async function activateDir(item) {
    show.value = false
    await fm.selectDirectory(props.manager, { path: item.path, history: true })
    if (fm.getManager(props.manager).error) {
        // The load error is already on screen via the axios interceptor —
        // just forget the dead entry.
        history.dropStale('dir', item.path)
    }
}

async function activateFile(item) {
    show.value = false
    const manager = fm.getManager(props.manager)
    const parent = item.dirname === '' ? null : item.dirname

    // Always (re)load the parent: only a fresh listing can tell a stale
    // entry from a live one.
    await fm.selectDirectory(props.manager, { path: parent, history: true })
    if (manager.error) {
        history.dropStale('file', item.path)
        return
    }

    // The raw listing, not the hiddenFiles-filtered getter: a dotfile from
    // the history must still open.
    const file = manager.files.find((f) => f.path === item.path)
    if (file) {
        openFile(file)
    } else {
        history.dropStale('file', item.path)
        notification({ content: lang.value.history.staleRemoved, type: 'info' })
    }
}
</script>

<style lang="scss">
.fm-history-btn {
    @apply inline-flex items-center justify-center px-2 py-0.5 rounded
        text-secondary hover:bg-white dark:hover:bg-stone-700
        transition-colors duration-100;
    flex: 0 0 auto;
}

.fm-history-btn--open {
    @apply bg-white dark:bg-stone-700 text-body;
}

.fm-history-pop {
    @apply p-1.5 bg-white dark:bg-stone-900 rounded-lg border shadow-lg text-sm text-body;
    width: min(320px, calc(100vw - 2rem));
}

.fm-history-tabs {
    @apply flex p-0.5 mb-1 rounded-md bg-stone-100 dark:bg-stone-800;
}

.fm-history-tab {
    @apply flex-1 py-0.5 rounded border border-transparent text-muted;
}

.fm-history-tab--active {
    @apply bg-white dark:bg-stone-900 border text-body font-medium;
}

.fm-history-section + .fm-history-section {
    @apply mt-1;
}

.fm-history-label {
    @apply px-2.5 pt-1.5 pb-0.5 text-xs text-faint;
}

.fm-history-item {
    @apply flex items-center gap-2 w-full px-2.5 py-1 rounded text-left;

    &:hover {
        @apply bg-surface-hover;
    }
}

.fm-history-icon {
    @apply text-muted;
    flex: none;
    width: 1.25em;
    text-align: center;
}

.fm-history-text {
    @apply min-w-0;
    flex: 1;
}

.fm-history-name,
.fm-history-path {
    @apply block truncate;
}

.fm-history-path {
    @apply text-xs text-faint;
}

.fm-history-meta {
    @apply text-xs text-faint whitespace-nowrap;
    flex: none;
}

.fm-history-empty {
    @apply px-2.5 py-4 text-center text-muted;

    small {
        @apply block mt-1 text-xs text-faint;
    }
}
</style>
