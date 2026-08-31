import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { viteCommonjs } from '@originjs/vite-plugin-commonjs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// PLUGIN_PATH was renamed to PLUGINS_PATH; the old name keeps working for one release.
function readPluginsPath(): string | undefined {
    if (!process.env.PLUGINS_PATH && process.env.PLUGIN_PATH) {
        console.warn('PLUGIN_PATH is deprecated and will be removed in a future release, use PLUGINS_PATH')
    }

    return process.env.PLUGINS_PATH || process.env.PLUGIN_PATH
}

// Default plugin path - can be overridden via PLUGINS_PATH env variable
function resolvePluginPath(): string {
    const pluginPath = readPluginsPath()

    if (!pluginPath) {
        // Default: no plugin loaded
        return resolve(__dirname, 'empty-plugin')
    }

    if (pluginPath.startsWith('/')) {
        // Absolute path
        return pluginPath
    }

    // Relative path from current working directory
    return resolve(process.cwd(), pluginPath)
}

export default defineConfig({
    plugins: [
        viteCommonjs(),
        vue(),
    ],
    root: __dirname,
    base: '/',
    publicDir: resolve(__dirname, 'public'),
    resolve: {
        alias: [
            // Debug harness source (for mocks, etc.)
            { find: '@debug', replacement: resolve(__dirname, 'src') },
            // Plugin source (built bundle from external plugin)
            { find: '@plugin', replacement: resolvePluginPath() },
        ],
    },
    css: {
        postcss: resolve(__dirname, 'postcss.config.cjs'),
        preprocessorOptions: {
            scss: {
                api: 'modern-compiler',
            },
        },
    },
    server: {
        port: 5174,
        open: true,
        fs: {
            // Allow serving files from anywhere (needed for npm packages)
            strict: false,
        },
    },
    optimizeDeps: {
        include: [
            'vue',
            'vue-router',
            'pinia',
            'axios',
            'naive-ui',
            'dayjs',
            'codemirror',
        ],
        // Don't pre-bundle these to allow proper resolution
        exclude: ['@gameap/plugin-sdk', '@gameap/frontend', 'msw'],
    },
    build: {
        outDir: 'dist',
        emptyOutDir: true,
    },
})
