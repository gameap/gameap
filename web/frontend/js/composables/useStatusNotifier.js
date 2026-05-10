import { useNotificationsStore } from '@/store/notifications'

export function useStatusNotifier() {
    const store = useNotificationsStore()

    return {
        show: (payload) => store.show(payload),
        dismiss: (id) => store.dismiss(id),
        clear: () => store.clear(),
    }
}
