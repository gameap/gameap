import { ref, onBeforeUnmount } from 'vue'
import { useWsStatusStore } from '@/store/wsStatus'

const MAX_RECONNECT_DELAY = 30000
const PING_INTERVAL = 25000
const PONG_TIMEOUT = PING_INTERVAL * 2

function buildWsUrl(path, token) {
    let base = import.meta.env.VITE_API_BASE_URL || window.location.origin
    base = base.replace(/^http/, 'ws')
    const separator = path.includes('?') ? '&' : '?'
    return `${base}${path}${separator}token=${encodeURIComponent(token)}`
}

function generateConnectionId() {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID()
    }

    return `ws-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export function useWebSocket(options = {}) {
    const {
        onMessage,
        onOpen,
        onClose,
        onError,
        reconnect = true,
        maxReconnectAttempts = 10,
    } = options

    const status = ref('disconnected')
    const wsStatusStore = useWsStatusStore()
    const connectionId = generateConnectionId()

    let ws = null
    let currentPath = null
    let reconnectAttempts = 0
    let reconnectTimer = null
    let pingTimer = null
    let pongTimer = null
    let lastPong = 0
    let shouldReconnect = reconnect
    let manualClose = false
    let registered = false

    function connect(urlPath) {
        const token = localStorage.getItem('auth_token')
        if (!token) {
            return
        }

        manualClose = false
        shouldReconnect = reconnect
        currentPath = urlPath

        cleanup()

        const url = buildWsUrl(urlPath, token)
        status.value = 'connecting'

        if (!registered) {
            wsStatusStore.register(connectionId)
            registered = true
        }
        wsStatusStore.setStatus(connectionId, 'connecting', reconnectAttempts)

        ws = new WebSocket(url)

        ws.onopen = () => {
            status.value = 'connected'
            reconnectAttempts = 0
            lastPong = Date.now()
            wsStatusStore.setStatus(connectionId, 'connected', 0)
            wsStatusStore.markEverConnected(connectionId)
            startPing()
            onOpen?.()
        }

        ws.onmessage = (event) => {
            let msg
            try {
                msg = JSON.parse(event.data)
            } catch (e) {
                console.warn('[WS] Failed to parse message:', event.data)
                return
            }

            if (msg.type === 'pong') {
                lastPong = Date.now()
                return
            }

            onMessage?.(msg)
        }

        ws.onclose = () => {
            status.value = 'disconnected'
            wsStatusStore.setStatus(connectionId, 'disconnected', reconnectAttempts)
            stopPing()
            onClose?.()

            if (shouldReconnect && !manualClose) {
                scheduleReconnect()
            }
        }

        ws.onerror = (event) => {
            onError?.(event)
        }
    }

    function send(type, payload, id) {
        if (!ws || ws.readyState !== WebSocket.OPEN) {
            return
        }

        const msg = { type }
        if (payload !== undefined) {
            msg.payload = payload
        }
        if (id !== undefined) {
            msg.id = id
        }

        ws.send(JSON.stringify(msg))
    }

    function close() {
        manualClose = true
        shouldReconnect = false
        cleanup()

        if (registered) {
            wsStatusStore.unregister(connectionId)
            registered = false
        }
    }

    function cleanup() {
        clearTimeout(reconnectTimer)
        reconnectTimer = null
        stopPing()

        if (ws) {
            ws.onopen = null
            ws.onmessage = null
            ws.onclose = null
            ws.onerror = null

            if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
                ws.close()
            }

            ws = null
        }

        status.value = 'disconnected'
    }

    function scheduleReconnect() {
        if (reconnectAttempts >= maxReconnectAttempts) {
            wsStatusStore.setStatus(connectionId, 'failed', reconnectAttempts)
            return
        }

        if (!localStorage.getItem('auth_token')) {
            return
        }

        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), MAX_RECONNECT_DELAY)
        reconnectAttempts++

        reconnectTimer = setTimeout(() => {
            if (currentPath && shouldReconnect) {
                connect(currentPath)
            }
        }, delay)
    }

    function startPing() {
        stopPing()
        pingTimer = setInterval(() => {
            send('ping')

            pongTimer = setTimeout(() => {
                if (Date.now() - lastPong > PONG_TIMEOUT) {
                    cleanup()
                    scheduleReconnect()
                }
            }, PONG_TIMEOUT)
        }, PING_INTERVAL)
    }

    function stopPing() {
        clearInterval(pingTimer)
        pingTimer = null
        clearTimeout(pongTimer)
        pongTimer = null
    }

    onBeforeUnmount(() => {
        close()
    })

    return {
        status,
        connect,
        send,
        close,
    }
}
