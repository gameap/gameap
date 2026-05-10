import { h, watch, onUnmounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useWsStatusStore } from '@/store/wsStatus'
import { useStatusNotifier } from '@/composables/useStatusNotifier'
import { trans } from '@/i18n/i18n'

export function useWsStatusNotifications(dialog) {
    const notifier = useStatusNotifier()
    const { aggregateStatus, failedFirstConnect, anyEverConnected, hasFailureSignal } = storeToRefs(useWsStatusStore())

    let currentId = null

    function dismiss() {
        if (currentId) {
            notifier.dismiss(currentId)
            currentId = null
        }
    }

    function openDetailsDialog() {
        dialog.info({
            title: trans('ws_status.proxy_hint_title'),
            style: 'max-width: 720px',
            content: () => h('div', [
                h('p', { class: 'mb-3' }, trans('ws_status.proxy_hint_text')),
                h('pre', {
                    class: 'text-xs whitespace-pre-wrap bg-stone-100 dark:bg-stone-800 p-3 rounded border border-stone-300 dark:border-stone-700 overflow-x-auto',
                }, trans('ws_status.proxy_hint_details')),
            ]),
            positiveText: trans('main.close'),
        })
    }

    function showProxyHint() {
        dismiss()
        currentId = notifier.show({
            type: 'error',
            title: trans('ws_status.proxy_hint_title'),
            text: trans('ws_status.proxy_hint_text'),
            actions: [{
                label: trans('ws_status.show_details'),
                onClick: openDetailsDialog,
            }],
        })
    }

    function showDisconnected() {
        dismiss()
        currentId = notifier.show({
            type: 'warning',
            text: trans('ws_status.disconnected_text'),
        })
    }

    watch(
        [aggregateStatus, failedFirstConnect, anyEverConnected, hasFailureSignal],
        ([status, failedFirst, everConnected, failureSignal]) => {
            if (status === 'connected' || status === 'idle') {
                dismiss()
            } else if (failedFirst) {
                showProxyHint()
            } else if (everConnected || failureSignal) {
                showDisconnected()
            } else {
                dismiss()
            }
        },
        { immediate: true },
    )

    onUnmounted(dismiss)
}
