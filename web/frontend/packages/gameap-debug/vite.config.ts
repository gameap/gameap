import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Custom plugin to resolve @gameap/plugin-sdk from anywhere
function pluginSdkResolver(sdkPath: string): Plugin {
    return {
        name: 'plugin-sdk-resolver',
        enforce: 'pre',
        resolveId(source) {
            if (source === '@gameap/plugin-sdk') {
                return sdkPath
            }
            return null
        },
    }
}

// Default plugin path - can be overridden via PLUGIN_PATH env variable
// If PLUGIN_PATH is relative, it's resolved from current working directory
// If PLUGIN_PATH is absolute, it's used as-is
function resolvePluginPath(): string {
    const pluginPath = process.env.PLUGIN_PATH

    if (!pluginPath) {
        // Default to hex-editor-plugin's built bundle
        // Path: gameap-debug -> packages -> frontend -> web -> gameap-api -> gameap -> hex-editor-plugin
        return resolve(__dirname, '../../../../../hex-editor-plugin/frontend/dist')
    }

    if (pluginPath.startsWith('/')) {
        // Absolute path
        return pluginPath
    }

    // Relative path from current working directory
    return resolve(process.cwd(), pluginPath)
}

// Use the built plugin-sdk to avoid resolution issues
const pluginSdkPath = resolve(__dirname, '../../plugin-sdk/dist/index.js')

export default defineConfig({
    plugins: [
        pluginSdkResolver(pluginSdkPath),
        vue(),
    ],
    root: __dirname,
    base: '/',
    resolve: {
        alias: [
            // Debug harness src
            { find: '@', replacement: resolve(__dirname, 'src') },
            // Plugin SDK - use regex to match from anywhere
            { find: /^@gameap\/plugin-sdk$/, replacement: pluginSdkPath },
            // Plugin source
            { find: '@plugin', replacement: resolvePluginPath() },
        ],
    },
    server: {
        port: 5174,
        open: true,
        fs: {
            // Allow serving files from these directories
            allow: [
                __dirname,
                resolve(__dirname, '../../plugin-sdk'),
                resolvePluginPath(),
                // Also allow node_modules
                resolve(__dirname, '../../node_modules'),
            ],
        },
    },
    define: {
        'window.gameapLang': JSON.stringify(process.env.LOCALE || 'en'),
    },
    optimizeDeps: {
        include: ['vue', 'vue-router', 'pinia'],
        // Don't pre-bundle the plugin SDK to allow alias resolution
        exclude: ['@gameap/plugin-sdk'],
        esbuildOptions: {
            // Ensure esbuild also resolves the SDK correctly
            alias: {
                '@gameap/plugin-sdk': pluginSdkPath,
            },
        },
    },
})
