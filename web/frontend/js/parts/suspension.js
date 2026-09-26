import {trans} from '@/i18n/i18n'

// suspensionDetails lists what is known about a suspension, since when and
// why, as lines to show next to the "suspended" status. A server blocked
// before the panel recorded the date has no date to show.
export function suspensionDetails(suspension) {
    const details = []

    if (suspension?.since) {
        const since = new Date(suspension.since)

        if (!Number.isNaN(since.getTime())) {
            details.push(trans('servers.suspended_since', {date: since.toLocaleString()}))
        }
    }

    if (suspension?.reason) {
        details.push(trans('servers.suspended_reason', {reason: suspension.reason}))
    }

    return details
}
