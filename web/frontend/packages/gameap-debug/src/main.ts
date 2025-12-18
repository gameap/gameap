/**
 * GameAP Debug Harness - Main Entry Point
 *
 * This harness loads the real GameAP frontend with mock API responses,
 * allowing plugin testing in a realistic environment.
 */

// Import frontend styles (bundled in vendor directory)
import '../vendor/frontend.css'

import { startMockServiceWorker, setPluginContent, updateDebugState } from './mocks/browser'

// Declare window globals for plugin compatibility
declare global {
    interface Window {
        Vue: typeof import('vue')
        VueRouter: typeof import('vue-router')
        Pinia: typeof import('pinia')
        axios: typeof import('axios').default
        gameapLang: string
        i18n: Record<string, string>
        gameapDebug: {
            updateDebugState: typeof updateDebugState
            setPluginContent: typeof setPluginContent
            loadPlugin: (js: string, css?: string) => void
        }
    }
}


// Load plugin from dist directory (set via PLUGIN_PATH env var)
// Using glob imports to handle dynamic file names
const pluginJsFiles = import.meta.glob('@plugin/plugin.js', { query: '?raw', import: 'default', eager: true })
const pluginCssFiles = import.meta.glob('@plugin/*.css', { query: '?raw', import: 'default', eager: true })

async function loadPluginBundle(): Promise<{ js: string; css: string }> {
    try {
        // Get plugin JS
        const jsEntries = Object.entries(pluginJsFiles)
        const pluginJs = jsEntries.length > 0 ? (jsEntries[0][1] as string) : ''

        // Get plugin CSS (any .css file in the dist)
        const cssEntries = Object.entries(pluginCssFiles)
        const pluginCss = cssEntries.length > 0 ? (cssEntries[0][1] as string) : ''

        if (!pluginJs) {
            console.warn('[Debug] No plugin.js found in plugin directory')
        }
        if (!pluginCss) {
            console.log('[Debug] No plugin CSS found')
        }

        return { js: pluginJs, css: pluginCss }
    } catch (error) {
        console.warn('[Debug] Could not load plugin bundle:', error)
        return { js: '', css: '' }
    }
}

// Set up mock authentication based on debug state
function setupMockAuth() {
    // Get stored debug user type or default to 'admin'
    const storedUserType = localStorage.getItem('gameap_debug_user_type') || 'admin'
    updateDebugState({ userType: storedUserType as 'admin' | 'user' | 'guest' })

    if (storedUserType === 'guest') {
        // Guest mode - remove auth token
        localStorage.removeItem('auth_token')
        console.log('[Debug] Auth: Guest mode (not authenticated)')
    } else {
        // Authenticated mode - set mock token
        const mockToken = 'mock-debug-token-' + storedUserType
        localStorage.setItem('auth_token', mockToken)
        console.log('[Debug] Auth: Authenticated as', storedUserType)
    }
}

// Load translations after MSW starts (since MSW intercepts the request)
async function loadTranslations() {
    const lang = window.gameapLang || 'en'
    console.log('[Debug] Loading translations for:', lang)

    try {
        const response = await fetch(`/lang/${lang}.json`)
        if (response.ok) {
            window.i18n = await response.json()
            console.log('[Debug] Translations loaded successfully')
        } else {
            console.warn('[Debug] Failed to load translations, using fallback')
            window.i18n = {}
        }
    } catch (error) {
        console.error('[Debug] Error loading translations:', error)
        window.i18n = {}
    }
}

// Initialize the debug harness
async function init() {
    console.log('[Debug] Starting GameAP Debug Harness...')

    // Load plugin bundle first
    console.log('[Debug] Loading plugin bundle...')
    const { js, css } = await loadPluginBundle()

    if (js) {
        console.log('[Debug] Plugin bundle loaded, size:', js.length, 'bytes')
        setPluginContent(js, css)
    }

    // Start MSW
    console.log('[Debug] Starting Mock Service Worker...')
    await startMockServiceWorker()
    console.log('[Debug] MSW started successfully')

    // Set up mock authentication BEFORE loading the app
    setupMockAuth()

    // Load translations AFTER MSW starts (so MSW can intercept the request)
    await loadTranslations()

    // Expose debug utilities globally
    window.gameapDebug = {
        updateDebugState,
        setPluginContent,
        loadPlugin: (newJs: string, newCss?: string) => {
            setPluginContent(newJs, newCss || '')
            console.log('[Debug] Plugin content updated, reload to apply')
        },
    }

    // Now load the real GameAP frontend
    console.log('[Debug] Loading GameAP frontend...')

    // Import the frontend from npm package - this will initialize the Vue app
    await import('@gameap/frontend')

    console.log('[Debug] GameAP frontend loaded successfully')

    // Add debug panel after app is mounted
    setTimeout(() => {
        createDebugPanel()
    }, 500)
}

// Create a floating debug panel for controlling the mock environment
function createDebugPanel() {
    const panel = document.createElement('div')
    panel.id = 'gameap-debug-panel'
    panel.innerHTML = `
        <style>
            #gameap-debug-panel {
                position: fixed;
                bottom: 20px;
                right: 20px;
                z-index: 99999;
                font-family: system-ui, sans-serif;
                font-size: 12px;
            }
            #gameap-debug-panel .debug-toggle {
                background: #4f46e5;
                color: white;
                border: none;
                padding: 8px 12px;
                border-radius: 8px;
                cursor: pointer;
                box-shadow: 0 4px 12px rgba(0,0,0,0.15);
                display: flex;
                align-items: center;
                gap: 6px;
            }
            #gameap-debug-panel .debug-toggle:hover {
                background: #4338ca;
            }
            #gameap-debug-panel .debug-content {
                display: none;
                position: absolute;
                bottom: 100%;
                right: 0;
                margin-bottom: 8px;
                background: white;
                border-radius: 12px;
                box-shadow: 0 4px 20px rgba(0,0,0,0.15);
                min-width: 280px;
                overflow: hidden;
            }
            #gameap-debug-panel.open .debug-content {
                display: block;
            }
            #gameap-debug-panel .debug-header {
                background: #4f46e5;
                color: white;
                padding: 12px 16px;
                font-weight: 600;
            }
            #gameap-debug-panel .debug-body {
                padding: 16px;
            }
            #gameap-debug-panel .debug-section {
                margin-bottom: 12px;
            }
            #gameap-debug-panel .debug-section:last-child {
                margin-bottom: 0;
            }
            #gameap-debug-panel label {
                display: block;
                font-size: 11px;
                font-weight: 500;
                color: #64748b;
                margin-bottom: 4px;
                text-transform: uppercase;
            }
            #gameap-debug-panel select, #gameap-debug-panel input {
                width: 100%;
                padding: 8px 10px;
                border: 1px solid #e2e8f0;
                border-radius: 6px;
                font-size: 13px;
            }
            #gameap-debug-panel select:focus, #gameap-debug-panel input:focus {
                outline: none;
                border-color: #4f46e5;
            }
            #gameap-debug-panel .debug-badge {
                background: #dbeafe;
                color: #1d4ed8;
                padding: 4px 8px;
                border-radius: 4px;
                font-size: 11px;
                margin-top: 4px;
                display: inline-block;
            }
            @media (prefers-color-scheme: dark) {
                #gameap-debug-panel .debug-content {
                    background: #1e293b;
                }
                #gameap-debug-panel label {
                    color: #94a3b8;
                }
                #gameap-debug-panel select, #gameap-debug-panel input {
                    background: #0f172a;
                    border-color: #334155;
                    color: #f1f5f9;
                }
                #gameap-debug-panel .debug-badge {
                    background: #1e3a5f;
                    color: #93c5fd;
                }
            }
        </style>
        <div class="debug-content">
            <div class="debug-header">🔧 Debug Panel</div>
            <div class="debug-body">
                <div class="debug-section">
                    <label>User Type</label>
                    <select id="debug-user-type">
                        <option value="admin">Admin</option>
                        <option value="user">Regular User</option>
                        <option value="guest">Guest (not authenticated)</option>
                    </select>
                </div>
                <div class="debug-section">
                    <label>Network Delay (ms)</label>
                    <input type="number" id="debug-network-delay" value="100" min="0" max="5000" step="50">
                </div>
                <div class="debug-section">
                    <label>Locale</label>
                    <select id="debug-locale">
                        <option value="en">English</option>
                        <option value="ru">Русский</option>
                    </select>
                </div>
                <div class="debug-section">
                    <span class="debug-badge">MSW Active</span>
                    <span class="debug-badge">Plugin Loaded</span>
                </div>
            </div>
        </div>
        <button class="debug-toggle">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="3"></circle>
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
            </svg>
            Debug
        </button>
    `

    document.body.appendChild(panel)

    // Toggle panel
    const toggleBtn = panel.querySelector('.debug-toggle') as HTMLButtonElement
    toggleBtn.addEventListener('click', () => {
        panel.classList.toggle('open')
    })

    // User type change
    const userTypeSelect = panel.querySelector('#debug-user-type') as HTMLSelectElement
    // Set current value from localStorage
    const currentUserType = localStorage.getItem('gameap_debug_user_type') || 'admin'
    userTypeSelect.value = currentUserType

    userTypeSelect.addEventListener('change', () => {
        const newUserType = userTypeSelect.value as 'admin' | 'user' | 'guest'
        // Save to localStorage for persistence
        localStorage.setItem('gameap_debug_user_type', newUserType)
        updateDebugState({ userType: newUserType })
        console.log('[Debug] User type changed to:', newUserType)
        // Reload to apply
        if (confirm('Reload page to apply user type change?')) {
            window.location.reload()
        }
    })

    // Network delay change
    const delayInput = panel.querySelector('#debug-network-delay') as HTMLInputElement
    delayInput.addEventListener('change', () => {
        updateDebugState({ networkDelay: parseInt(delayInput.value) || 100 })
        console.log('[Debug] Network delay set to:', delayInput.value, 'ms')
    })

    // Locale change
    const localeSelect = panel.querySelector('#debug-locale') as HTMLSelectElement
    localeSelect.addEventListener('change', () => {
        updateDebugState({ locale: localeSelect.value as 'en' | 'ru' })
        window.gameapLang = localeSelect.value
        console.log('[Debug] Locale changed to:', localeSelect.value)
    })
}

// Start initialization
init().catch(error => {
    console.error('[Debug] Failed to initialize:', error)
})
