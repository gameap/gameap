import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

import POST from '../http/post.js'

const COMPLETED_REMOVE_DELAY = 5000
const CANCELLED_REMOVE_DELAY = 3000

const ACTIVE_STATUSES = ['pending', 'running', 'cancelling']

/**
 * Tracks server-side archive create/extract operations started from this
 * tab. State lives in memory only: a reload loses the list while the daemon
 * finishes the operation on its own.
 */
export const useArchiveOperationsStore = defineStore('fm-archive-ops', () => {
    const operations = ref([])

    const removalTimers = new Map()

    const activeOperations = computed(() =>
        operations.value.filter((op) => ACTIVE_STATUSES.includes(op.status)),
    )
    const hasActive = computed(() => activeOperations.value.length > 0)
    const visibleOperations = computed(() => operations.value)

    function find(id) {
        return operations.value.find((op) => op.id === id) || null
    }

    function add({ id, type, label, disk }) {
        operations.value.push({
            id,
            type,
            label,
            disk: disk ?? null,
            status: 'pending',
            filesProcessed: 0,
            filesTotal: 0,
            bytesProcessed: 0,
            bytesTotal: 0,
            currentEntry: '',
            error: null,
            startedAt: Date.now(),
            lastEventAt: Date.now(),
        })
    }

    function applyProgress(payload) {
        const op = find(payload?.operation_id)
        if (!op) return null

        op.filesProcessed = payload.files_processed ?? 0
        op.filesTotal = payload.files_total ?? 0
        op.bytesProcessed = payload.bytes_processed ?? 0
        op.bytesTotal = payload.bytes_total ?? 0
        op.currentEntry = payload.current_entry ?? ''
        op.lastEventAt = Date.now()

        if (op.status === 'pending' || op.status === 'stale') {
            op.status = 'running'
        }

        return op
    }

    function applyComplete(payload) {
        const op = find(payload?.operation_id)
        if (!op) return null

        op.filesProcessed = payload.files_processed ?? op.filesProcessed
        op.bytesProcessed = payload.bytes_processed ?? op.bytesProcessed
        op.currentEntry = ''
        op.lastEventAt = Date.now()

        if (payload.success) {
            op.status = 'completed'
            scheduleRemoval(op.id, COMPLETED_REMOVE_DELAY)
        } else if (String(payload.error || '').startsWith('canceled')) {
            op.status = 'cancelled'
            scheduleRemoval(op.id, CANCELLED_REMOVE_DELAY)
        } else {
            op.status = 'error'
            op.error = payload.error || null
        }

        return op
    }

    async function cancelOperation(id) {
        const op = find(id)
        if (!op || !ACTIVE_STATUSES.includes(op.status)) return

        const previous = op.status
        op.status = 'cancelling'

        try {
            await POST.cancelArchiveOperation(id, { silentError: true })
        } catch (e) {
            // The final WS event stays the source of truth either way.
            const current = find(id)
            if (current && current.status === 'cancelling') {
                current.status = previous
            }
        }
    }

    function markStale() {
        operations.value.forEach((op) => {
            if (ACTIVE_STATUSES.includes(op.status)) {
                op.status = 'stale'
            }
        })
    }

    function scheduleRemoval(id, delay) {
        clearTimeout(removalTimers.get(id))
        removalTimers.set(id, setTimeout(() => remove(id), delay))
    }

    function remove(id) {
        clearTimeout(removalTimers.get(id))
        removalTimers.delete(id)
        operations.value = operations.value.filter((op) => op.id !== id)
    }

    function clear() {
        removalTimers.forEach((timer) => clearTimeout(timer))
        removalTimers.clear()
        operations.value = []
    }

    return {
        operations,
        activeOperations,
        hasActive,
        visibleOperations,
        add,
        applyProgress,
        applyComplete,
        cancelOperation,
        markStale,
        remove,
        clear,
    }
})
