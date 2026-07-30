import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Every color below resolves through the --gameap-* variables defined in
// packages/gameap-ui/theme.css (shipped as /theme.css), so plugins can
// re-theme the panel by overriding the variables at runtime.
const SHADES = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]

const varScale = (family) =>
    Object.fromEntries(SHADES.map((s) => [s, `var(--gameap-${family}-${s})`]))

const intent = (name) => ({
    DEFAULT: `var(--gameap-${name})`,
    hover: `var(--gameap-${name}-hover)`,
    soft: `var(--gameap-${name}-soft)`,
    'soft-text': `var(--gameap-${name}-soft-text)`,
})

const chartColors = Object.fromEntries(
    Array.from({ length: 10 }, (_, i) => [`chart-${i + 1}`, `var(--gameap-chart-${i + 1})`])
)

export default {
    content: [
        resolve(__dirname, 'index.html'),
        resolve(__dirname, 'js/**/*.{js,ts,jsx,tsx,vue}'),
        resolve(__dirname, 'packages/gameap-ui/**/*.{js,vue}'),
        resolve(__dirname, 'packages/gameap-debug/src/**/*.{js,vue}'),
        resolve(__dirname, 'packages/gameap-frontend/*.{js,vue}'),
    ],
    // Semantic utilities guaranteed to exist for plugin templates: plugin CSS
    // is compiled outside this build, so plugins can only use classes the
    // panel's compiled CSS already contains.
    safelist: [
        'bg-surface', 'bg-surface-raised', 'bg-surface-overlay', 'bg-surface-hover',
        'bg-scrim', 'bg-terminal', 'bg-chrome', 'bg-chrome-item',
        'bg-primary', 'bg-primary-hover', 'bg-primary-soft',
        'bg-success', 'bg-success-soft',
        'bg-danger', 'bg-danger-hover', 'bg-danger-soft',
        'bg-warning', 'bg-warning-soft',
        'bg-info', 'bg-info-hover', 'bg-info-soft',
        'text-body', 'text-secondary', 'text-muted', 'text-faint',
        'text-primary', 'text-success', 'text-danger', 'text-warning', 'text-info',
        'text-primary-soft-text', 'text-success-soft-text', 'text-danger-soft-text',
        'text-warning-soft-text', 'text-info-soft-text',
        'border-strong', 'border-danger', 'border-warning',
    ],
    theme: {
        extend: {
            colors: {
                stone: varScale('stone'),
                red: varScale('red'),
                orange: varScale('orange'),
                lime: varScale('lime'),
                sky: varScale('sky'),
                primary: intent('primary'),
                success: intent('success'),
                danger: intent('danger'),
                warning: intent('warning'),
                info: intent('info'),
                chrome: {
                    DEFAULT: 'var(--gameap-chrome)',
                    item: 'var(--gameap-chrome-item)',
                    hover: 'var(--gameap-chrome-hover)',
                },
                ...chartColors,
            },
            backgroundColor: {
                surface: 'var(--gameap-surface)',
                'surface-raised': 'var(--gameap-surface-raised)',
                'surface-overlay': 'var(--gameap-surface-overlay)',
                'surface-hover': 'var(--gameap-surface-hover)',
                scrim: 'var(--gameap-scrim)',
                terminal: 'var(--gameap-terminal-bg)',
            },
            textColor: {
                body: 'var(--gameap-text)',
                secondary: 'var(--gameap-text-secondary)',
                muted: 'var(--gameap-text-muted)',
                faint: 'var(--gameap-text-faint)',
                terminal: 'var(--gameap-terminal-text)',
            },
            borderColor: {
                DEFAULT: 'var(--gameap-border)',
                strong: 'var(--gameap-border-strong)',
            },
        },
    },
    plugins: [
        require('@tailwindcss/aspect-ratio'),
    ],
    darkMode: 'selector',
}
