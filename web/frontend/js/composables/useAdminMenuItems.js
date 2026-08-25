import { computed } from 'vue'
import { adminLinks } from '@/components/bars'
import { usePluginsStore } from '@/store/plugins'

const PLUGINS_LINK_ROUTE_NAME = 'admin.plugins.index'

// Admin menu: core links merged with plugin-provided items.
// Plugin items are inserted before the "Plugins" link so it always stays last.
export function useAdminMenuItems() {
    const pluginsStore = usePluginsStore()

    return computed(() => {
        const links = adminLinks.map((link) => ({
            key: link.route.name,
            icon: link.icon,
            text: link.text,
            route: link.route,
        }))

        const pluginItems = pluginsStore.getMenuItems('admin').map((item) => ({
            key: item.pluginId + '-' + item.text,
            icon: item.icon,
            text: pluginsStore.resolvePluginText(item.pluginId, item.text),
            route: item.route,
        }))

        if (pluginItems.length === 0) {
            return links
        }

        const insertAt = links.findIndex((link) => link.route.name === PLUGINS_LINK_ROUTE_NAME)
        if (insertAt === -1) {
            return [...links, ...pluginItems]
        }

        links.splice(insertAt, 0, ...pluginItems)

        return links
    })
}
