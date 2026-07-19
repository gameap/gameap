import { ref, onMounted, onUnmounted } from 'vue'

export function useIsSmallScreen(breakpoint = 768) {
    const isSmallScreen = ref(window.innerWidth < breakpoint)

    const handleResize = () => {
        isSmallScreen.value = window.innerWidth < breakpoint
    }

    onMounted(() => {
        window.addEventListener('resize', handleResize)
    })

    onUnmounted(() => {
        window.removeEventListener('resize', handleResize)
    })

    return isSmallScreen
}
