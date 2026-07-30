import { computed } from 'vue'
import { useUISettingsStore } from '../store/uiSettings'

export function readThemeVar(name, fallback = '') {
    const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()

    return value || fallback
}

// Reactive access to the --gameap-* theme variables for code that needs
// resolved color strings (ECharts, naive-ui props). Values re-resolve when
// the light/dark theme switches.
export function useThemeVars() {
    const uiSettingsStore = useUISettingsStore()

    const themeVar = (name, fallback = '') => {
        void uiSettingsStore.currentTheme

        return readThemeVar(name, fallback)
    }

    const chartPalette = computed(() => {
        void uiSettingsStore.currentTheme

        return Array.from({ length: 10 }, (_, i) => readThemeVar(`--gameap-chart-${i + 1}`))
    })

    const statusColors = computed(() => {
        void uiSettingsStore.currentTheme

        return {
            primary: readThemeVar('--gameap-primary', '#84cc16'),
            success: readThemeVar('--gameap-success', '#84cc16'),
            danger: readThemeVar('--gameap-danger', '#ef4444'),
            warning: readThemeVar('--gameap-warning', '#fb923c'),
            info: readThemeVar('--gameap-info', '#0ea5e9'),
        }
    })

    return { themeVar, chartPalette, statusColors }
}
