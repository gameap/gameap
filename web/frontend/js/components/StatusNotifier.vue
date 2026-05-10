<template></template>

<script setup>
import { h, watchEffect, onMounted, onUnmounted } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import { useNotificationsStore } from '@/store/notifications'
import { useWsStatusNotifications } from '@/composables/useWsStatusNotifications'

const message = useMessage()
const dialog = useDialog()
const store = useNotificationsStore()

const messageMap = new Map()

function renderActions(actions) {
    return h(
        'div',
        { class: 'flex gap-3 shrink-0' },
        actions.map(action =>
            h('a', {
                class: 'underline text-sm cursor-pointer whitespace-nowrap',
                onClick: action.onClick,
            }, action.label),
        ),
    )
}

function renderContent(notification) {
    const titleNode = notification.title
        ? h('div', { class: 'font-semibold' }, notification.title)
        : null
    const textNode = notification.text
        ? h('div', { class: 'text-sm' }, notification.text)
        : null

    const contentNodes = [titleNode, textNode].filter(Boolean)

    if (notification.actions?.length) {
        return h('div', { class: 'flex items-center gap-3' }, [
            h('div', { class: 'min-w-0 flex-1' }, contentNodes),
            renderActions(notification.actions),
        ])
    }

    return h('div', contentNodes)
}

function createMessage(notification) {
    const options = {
        type: notification.type ?? 'info',
        duration: notification.duration ?? 0,
        closable: notification.closable ?? true,
        onClose: () => store.dismiss(notification.id),
    }

    if (typeof notification.render === 'function') {
        options.render = notification.render
        return message.create('', options)
    }

    return message.create(() => renderContent(notification), options)
}

function syncMessages() {
    const current = store.notifications
    const currentIds = new Set(current.map(n => n.id))

    for (const [id, msg] of messageMap.entries()) {
        if (!currentIds.has(id)) {
            msg.destroy()
            messageMap.delete(id)
        }
    }

    for (const n of current) {
        if (!messageMap.has(n.id)) {
            messageMap.set(n.id, createMessage(n))
        }
    }
}

watchEffect(syncMessages)

onMounted(() => {
    window.$notifier = {
        show: (payload) => store.show(payload),
        dismiss: (id) => store.dismiss(id),
        clear: () => store.clear(),
        list: () => [...store.notifications],
        info: (text, opts = {}) => store.show({ type: 'info', text, ...opts }),
        success: (text, opts = {}) => store.show({ type: 'success', text, ...opts }),
        warning: (text, opts = {}) => store.show({ type: 'warning', text, ...opts }),
        error: (text, opts = {}) => store.show({ type: 'error', text, ...opts }),
        loading: (text, opts = {}) => store.show({ type: 'loading', text, ...opts }),
    }
})

onUnmounted(() => {
    delete window.$notifier
    for (const msg of messageMap.values()) {
        msg.destroy()
    }
    messageMap.clear()
})

useWsStatusNotifications(dialog)
</script>
