import mitt from 'mitt'

const emitter = mitt()

export function createPluginEventBus(pluginId) {
    return {
        emit(event, data) {
            emitter.emit(`plugin:${pluginId}:${event}`, data)
            emitter.emit(`plugin:*:${event}`, { pluginId, data })
        },

        on(event, handler) {
            emitter.on(`plugin:${pluginId}:${event}`, handler)
        },

        off(event, handler) {
            emitter.off(`plugin:${pluginId}:${event}`, handler)
        },

        onGlobal(event, handler) {
            emitter.on(`plugin:*:${event}`, handler)
        },

        offGlobal(event, handler) {
            emitter.off(`plugin:*:${event}`, handler)
        }
    }
}

export { emitter as pluginEmitter }
