import { computed } from 'vue'
import { useFileManagerStore } from '../stores/useFileManagerStore.js'
import { useSettingsStore } from '../stores/useSettingsStore.js'
import { useMessagesStore } from '../stores/useMessagesStore.js'
import { useModalStore } from '../stores/useModalStore.js'
import { useManager } from './useManager.js'

/**
 * Unified composable for accessing all filemanager stores
 * Provides convenient access to all stores and managers
 */
export function useFileManager() {
    const fm = useFileManagerStore()
    const settings = useSettingsStore()
    const messages = useMessagesStore()
    const modal = useModalStore()

    // Manager composable
    const manager = useManager('left')

    // Active manager composable (dynamic)
    const activeManagerName = computed(() => fm.activeManager)
    const activeManager = computed(() => useManager(fm.activeManager))

    return {
        // Stores
        fm,
        settings,
        messages,
        modal,
        // Manager composable
        manager,
        activeManagerName,
        activeManager,
    }
}
