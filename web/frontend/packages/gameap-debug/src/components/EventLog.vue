<template>
    <div class="flex flex-col bg-stone-50 dark:bg-stone-900 border-t border-stone-200 dark:border-stone-700">
        <!-- Header -->
        <div class="flex items-center justify-between p-2 border-b border-stone-200 dark:border-stone-700">
            <h4 class="text-sm font-semibold text-stone-700 dark:text-stone-300">
                <i class="fa-solid fa-clock-rotate-left mr-2"></i>
                {{ t('debug.events') }}
                <span v-if="events.length" class="text-xs text-stone-500">({{ events.length }})</span>
            </h4>
            <button
                v-if="events.length"
                @click="clearEvents"
                class="text-xs text-stone-500 hover:text-stone-700 dark:hover:text-stone-300"
            >
                <i class="fa-solid fa-trash mr-1"></i>
                Clear
            </button>
        </div>

        <!-- Events List -->
        <div class="flex-1 overflow-y-auto max-h-48">
            <div v-if="events.length === 0" class="p-4 text-center text-stone-500 dark:text-stone-400 text-sm">
                {{ t('debug.no_events') }}
            </div>

            <div v-else class="divide-y divide-stone-200 dark:divide-stone-700">
                <div
                    v-for="(event, index) in events"
                    :key="index"
                    class="p-2 hover:bg-stone-100 dark:hover:bg-stone-800"
                >
                    <div class="flex items-center justify-between">
                        <span
                            :class="eventTypeClass(event.type)"
                            class="text-xs font-semibold px-2 py-0.5 rounded"
                        >
                            {{ event.type }}
                        </span>
                        <span class="text-xs text-stone-400">
                            {{ formatTime(event.timestamp) }}
                        </span>
                    </div>
                    <div v-if="event.payload" class="mt-1 text-xs text-stone-600 dark:text-stone-400 font-mono truncate">
                        {{ formatPayload(event.payload) }}
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { useDebugStore, type DebugEvent } from '@/stores/debug'

defineProps<{
    events: DebugEvent[]
}>()

const debugStore = useDebugStore()

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}

function clearEvents() {
    debugStore.clearEventLog()
}

function eventTypeClass(type: string): string {
    switch (type) {
        case 'save':
            return 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
        case 'close':
            return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300'
        case 'error':
            return 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
        default:
            return 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
    }
}

function formatTime(date: Date): string {
    return date.toLocaleTimeString()
}

function formatPayload(payload: unknown): string {
    if (payload instanceof ArrayBuffer) {
        return `[ArrayBuffer: ${payload.byteLength} bytes]`
    }
    if (typeof payload === 'string') {
        return payload.length > 100 ? payload.slice(0, 100) + '...' : payload
    }
    try {
        const str = JSON.stringify(payload)
        return str.length > 100 ? str.slice(0, 100) + '...' : str
    } catch {
        return String(payload)
    }
}
</script>
