import { defineStore } from 'pinia'

let nextId = 0

function generateId() {
    nextId++

    return `notif-${nextId}`
}

export const useNotificationsStore = defineStore('notifications', {
    state: () => ({
        notifications: [],
    }),
    actions: {
        show(payload) {
            const id = payload.id || generateId()
            this.notifications.push({
                id,
                type: 'info',
                duration: 0,
                closable: true,
                ...payload,
            })

            return id
        },
        dismiss(id) {
            const index = this.notifications.findIndex(n => n.id === id)
            if (index !== -1) {
                this.notifications.splice(index, 1)
            }
        },
        clear() {
            this.notifications = []
        },
    },
})
