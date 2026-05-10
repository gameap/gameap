<template></template>

<script setup>
import { h, watchEffect, onUnmounted } from 'vue'
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

function renderNotification(notification) {
    if (typeof notification.render === 'function') {
        return notification.render()
    }

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
    return message.create('', {
        type: notification.type ?? 'info',
        duration: notification.duration ?? 0,
        closable: notification.closable ?? true,
        render: () => renderNotification(notification),
        onClose: () => store.dismiss(notification.id),
    })
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

onUnmounted(() => {
    for (const msg of messageMap.values()) {
        msg.destroy()
    }
    messageMap.clear()
})

useWsStatusNotifications(dialog)
</script>
