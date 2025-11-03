import { onBeforeUnmount } from 'vue'
import { requestCancellation } from '@/config/requestCancellation'

export function useRequestCancellation() {
    const abortController = new AbortController()

    onBeforeUnmount(() => {
        abortController.abort()
    })

    return {
        signal: abortController.signal,
        abort: () => abortController.abort(),
    }
}

export { requestCancellation }
