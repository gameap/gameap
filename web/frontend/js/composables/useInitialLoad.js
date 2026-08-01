import { onMounted, ref } from 'vue'

// Page-level flag for the first data load. Store `loading` getters are shared request
// counters: a page that chains several requests sees them flip between the steps, and
// the content blinks. This flag starts true — so the first paint is already the
// placeholder — and drops once, after the whole initial chain has settled.
export function useInitialLoad(load) {
    const loading = ref(true)

    onMounted(async () => {
        try {
            await load()
        } finally {
            loading.value = false
        }
    })

    return loading
}
