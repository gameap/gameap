<template>
    <div class="fm-breadcrumb" v-bind:class="{ 'fm-breadcrumb--active': manager === activeManager }">
        <nav class="fm-breadcrumb-nav" aria-label="breadcrumb">
            <button
                type="button"
                class="fm-breadcrumb-disk"
                v-bind:title="selectedDisk || ''"
                v-on:click="selectMainDirectory"
            >
                <GIcon name="hard-drive" />
                <span v-if="selectedDisk" class="fm-breadcrumb-disk-label">{{ selectedDisk }}</span>
            </button>

            <ol class="fm-breadcrumb-list">
                <template v-for="(item, index) in displaySegments" v-bind:key="index">
                    <li v-if="item.type === 'ellipsis'" class="fm-breadcrumb-sep">
                        <span class="fm-breadcrumb-divider">/</span>
                        <span class="fm-breadcrumb-ellipsis" v-bind:title="item.title">…</span>
                    </li>
                    <li
                        v-else
                        class="fm-breadcrumb-item"
                        v-bind:class="{ 'fm-breadcrumb-item--active': item.isLast }"
                        v-on:click="handleSelectDirectory(item.absoluteIndex)"
                    >
                        <span class="fm-breadcrumb-divider">/</span>
                        <span class="fm-breadcrumb-text">{{ item.label }}</span>
                    </li>
                </template>
            </ol>
        </nav>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useManager } from '../../composables/useManager.js'

const props = defineProps({
    manager: { type: String, required: true },
})

const fm = useFileManagerStore()
const { selectedDisk, selectedDirectory, breadcrumb, selectDirectory } = useManager(props.manager)

const activeManager = computed(() => fm.activeManager)

const MAX_VISIBLE = 5

const displaySegments = computed(() => {
    const segments = breadcrumb.value || []
    if (segments.length === 0) return []

    const last = segments.length - 1
    if (segments.length <= MAX_VISIBLE) {
        return segments.map((label, idx) => ({
            type: 'segment',
            label,
            absoluteIndex: idx,
            isLast: idx === last,
        }))
    }

    // Show first segment, ellipsis, then last (MAX_VISIBLE - 2) segments
    const tailCount = MAX_VISIBLE - 2
    const tailStart = segments.length - tailCount
    const collapsedSegments = segments.slice(1, tailStart)

    const result = [
        {
            type: 'segment',
            label: segments[0],
            absoluteIndex: 0,
            isLast: false,
        },
        {
            type: 'ellipsis',
            title: collapsedSegments.join('/'),
        },
    ]

    for (let i = tailStart; i < segments.length; i += 1) {
        result.push({
            type: 'segment',
            label: segments[i],
            absoluteIndex: i,
            isLast: i === last,
        })
    }

    return result
})

function handleSelectDirectory(index) {
    const segments = breadcrumb.value || []
    if (index < 0 || index >= segments.length) return

    const path = segments.slice(0, index + 1).join('/')
    if (path !== selectedDirectory.value) {
        selectDirectory(path, true)
    }
}

function selectMainDirectory() {
    if (selectedDirectory.value) {
        selectDirectory(null, true)
    }
}
</script>

<style lang="scss">
.fm-breadcrumb {
    @apply mb-2;
}

.fm-breadcrumb-nav {
    @apply flex items-center gap-1 px-1.5 py-1 rounded-md
        bg-stone-100 dark:bg-stone-800 border
        min-w-0;
    transition: border-color 120ms ease, background-color 120ms ease;
}

.fm-breadcrumb--active .fm-breadcrumb-nav {
    @apply border-stone-400 dark:border-stone-500;
}

.fm-breadcrumb-disk {
    @apply inline-flex items-center gap-1.5 px-2 py-0.5 rounded
        text-stone-700 dark:text-stone-200 text-sm font-medium
        hover:bg-white dark:hover:bg-[color:color-mix(in_srgb,var(--gameap-stone-700)_50%,transparent)]
        transition-colors duration-100;
    flex: 0 0 auto;
}

.fm-breadcrumb-disk-label {
    @apply text-xs uppercase tracking-wide text-muted;
}

.fm-breadcrumb-list {
    @apply flex items-center min-w-0;
    flex: 1 1 auto;
    overflow: hidden;
}

.fm-breadcrumb-item,
.fm-breadcrumb-sep {
    @apply inline-flex items-center min-w-0;
}

.fm-breadcrumb-divider {
    @apply mx-1 text-stone-400 dark:text-stone-600 text-sm select-none;
}

.fm-breadcrumb-text {
    @apply px-1.5 py-0.5 rounded text-sm text-secondary;
    cursor: pointer;
    transition: background-color 120ms ease, color 120ms ease;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 12rem;
}

.fm-breadcrumb-item:not(.fm-breadcrumb-item--active) .fm-breadcrumb-text:hover {
    @apply bg-white dark:bg-[color:color-mix(in_srgb,var(--gameap-stone-700)_50%,transparent)] text-body;
}

.fm-breadcrumb-item--active .fm-breadcrumb-text {
    @apply font-semibold text-stone-900 dark:text-stone-100;
    cursor: default;
}

.fm-breadcrumb-ellipsis {
    @apply px-1.5 text-faint select-none;
}
</style>
