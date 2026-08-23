import { trans } from '@/i18n/i18n'

const healthClasses = {
    healthy: 'bg-success-soft text-success-soft-text',
    degraded: 'bg-warning-soft text-warning-soft-text',
    unhealthy: 'bg-danger-soft text-danger-soft-text',
}

// Health is what the plugin last reported through gameap-host on the
// answering instance; a plugin that never reported has no badge.
export function pluginHealthBadge(health) {
    if (!health?.status || !healthClasses[health.status]) {
        return null
    }

    const details = Object.entries(health.details ?? {})
        .map(([key, value]) => `${key}: ${value}`)

    return {
        status: health.status,
        label: trans('plugins.health_' + health.status),
        message: health.message || '',
        class: healthClasses[health.status],
        title: [health.message, ...details].filter(Boolean).join('\n') || undefined,
    }
}
