import { watch, onUnmounted } from 'vue'
import { useWebSocket } from '@/composables/useWebSocket.js'
import { notification, errorNotification } from '@/parts/dialogs.js'
import { useArchiveOperationsStore } from '../stores/useArchiveOperationsStore.js'
import { useFileManagerStore } from '../stores/useFileManagerStore.js'
import { useSettingsStore } from '../stores/useSettingsStore.js'
import { useTranslate } from './useTranslate.js'

// The single backend integration point for the archive-progress stream.
export const archiveOpsWsPath = (serverId) =>
    `/api/ws/servers/${serverId}/file-manager/archive-operations`

const CONNECT_WAIT_TIMEOUT = 3000
const STALE_AFTER = 45000
const WATCHDOG_INTERVAL = 10000

/**
 * Owns the archive-operations WebSocket of the mounted file manager. Must be
 * called once from FileManager.vue setup (useWebSocket registers lifecycle
 * hooks). The socket connects before the first operation starts and closes
 * once no operation is active.
 */
export function useArchiveOperationsSocket() {
    const ops = useArchiveOperationsStore()
    const fm = useFileManagerStore()
    const settings = useSettingsStore()
    const { lang } = useTranslate()

    let watchdogTimer = null

    const ws = useWebSocket({
        onMessage(msg) {
            if (msg.type === 'archive.progress') {
                ops.applyProgress(msg.payload)
            } else if (msg.type === 'archive.complete') {
                handleComplete(msg.payload)
            }
        },
    })

    function serverId() {
        if (settings.serverId) return String(settings.serverId)

        // Fallback for embedders that only pass baseUrl ('/api/file-manager/3').
        return String(settings.baseUrl || '')
            .replace(/\/+$/, '')
            .split('/')
            .pop()
    }

    function handleComplete(payload) {
        const op = ops.applyComplete(payload)

        // Partial output exists even on error or cancel — always refresh.
        fm.refreshManagers()

        if (!op) return

        if (op.status === 'completed') {
            notification({
                content: op.type === 'archive'
                    ? lang.value.notifications.archiveCreated
                    : lang.value.notifications.archiveExtracted,
                type: 'success',
            })
        } else if (op.status === 'cancelled') {
            notification({ content: lang.value.progress.operationCancelled, type: 'info' })
        } else if (op.status === 'error') {
            const message = lang.value.response[op.error] ?? op.error ?? lang.value.progress.operationFailed
            errorNotification(message)
        }
    }

    // Connects (when needed) and waits for the socket to open so events of
    // fast operations are not lost between the 202 and the subscription. On
    // timeout the caller proceeds anyway — the watchdog covers the rest.
    async function connectAndWait(timeoutMs = CONNECT_WAIT_TIMEOUT) {
        if (ws.status.value === 'connected') return true

        if (ws.status.value === 'disconnected') {
            ws.connect(archiveOpsWsPath(serverId()))
        }

        const deadline = Date.now() + timeoutMs
        while (Date.now() < deadline) {
            if (ws.status.value === 'connected') return true
            // eslint-disable-next-line no-await-in-loop
            await new Promise((resolve) => setTimeout(resolve, 50))
        }

        return ws.status.value === 'connected'
    }

    function runWatchdog() {
        if (!ops.hasActive || ws.status.value === 'connected') return

        const lastEvent = Math.max(
            0,
            ...ops.activeOperations.map((op) => op.lastEventAt || 0),
        )
        if (lastEvent > 0 && Date.now() - lastEvent > STALE_AFTER) {
            ops.markStale()
        }
    }

    // The socket lives while any operation is unresolved — including stale
    // ones: a reconnect can still deliver their archive.complete and revive
    // them via applyProgress. It closes only once every operation is
    // terminal or dismissed.
    watch(
        () => ops.hasUnresolved,
        (unresolved) => {
            if (unresolved) {
                if (ws.status.value === 'disconnected') {
                    ws.connect(archiveOpsWsPath(serverId()))
                }
                if (!watchdogTimer) {
                    watchdogTimer = setInterval(runWatchdog, WATCHDOG_INTERVAL)
                }
            } else {
                ws.close()
                clearInterval(watchdogTimer)
                watchdogTimer = null
            }
        },
    )

    onUnmounted(() => {
        clearInterval(watchdogTimer)
        watchdogTimer = null
    })

    return {
        status: ws.status,
        connectAndWait,
    }
}
