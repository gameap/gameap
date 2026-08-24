import { trans } from '@/i18n/i18n'

const healthColors = {
    healthy: { badgeColor: 'green', alertType: 'success' },
    degraded: { badgeColor: 'orange', alertType: 'warning' },
    unhealthy: { badgeColor: 'red', alertType: 'error' },
}

// Health is what the plugin last reported through gameap-host on the
// answering instance; a plugin that never reported has no badge.
export function pluginHealthBadge(health) {
    if (!health?.status || !healthColors[health.status]) {
        return null
    }

    const details = Object.entries(health.details ?? {})
        .map(([key, value]) => `${key}: ${value}`)

    return {
        status: health.status,
        label: trans('plugins.health_' + health.status),
        message: health.message || '',
        ...healthColors[health.status],
        title: [health.message, ...details].filter(Boolean).join('\n') || undefined,
    }
}
