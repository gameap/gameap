import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { viteCommonjs } from '@originjs/vite-plugin-commonjs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import { getFileByPath } from './src/mocks/files'

const __dirname = dirname(fileURLToPath(import.meta.url))

// PLUGIN_PATH was renamed to PLUGINS_PATH; the old name keeps working for one release.
function readPluginsPath(): string | undefined {
    if (!process.env.PLUGINS_PATH && process.env.PLUGIN_PATH) {
        console.warn('PLUGIN_PATH is deprecated and will be removed in a future release, use PLUGINS_PATH')
    }

    return process.env.PLUGINS_PATH || process.env.PLUGIN_PATH
}

// The file manager hands downloads to the browser as navigations, which the MSW worker lets through
// to the network, so the dev server answers them from the same mock file tree. Archives are not mocked.
function mockFileManagerDownloads(): Plugin {
    return {
        name: 'gameap-debug-file-manager-downloads',
        configureServer(server) {
            server.middlewares.use((req, res, next) => {
                const url = new URL(req.url ?? '/', 'http://localhost')
                const match = /^\/api\/file-manager\/[^/]+\/(download|download-archive)$/.exec(url.pathname)
                if (!match) {
                    next()

                    return
                }

                const fail = (status: number, message: string) => {
                    res.statusCode = status
                    res.setHeader('Content-Type', 'application/json')
                    res.end(JSON.stringify({ status: 'error', message, http_code: status }))
                }

                if (match[1] === 'download-archive') {
                    fail(501, 'archive downloads are not mocked in the debug harness')

                    return
                }

                const path = url.searchParams.get('path') ?? ''
                const file = getFileByPath(path)
                if (!file || file._content === undefined) {
                    fail(404, `file not found: ${path}`)

                    return
                }

                const body = typeof file._content === 'string'
                    ? Buffer.from(file._content, 'utf8')
                    : Buffer.from(file._content)
                const name = encodeURIComponent(path.split('/').pop() || 'file')
                res.statusCode = 200
                res.setHeader('Content-Type', 'application/octet-stream')
                res.setHeader('Content-Disposition', `attachment; filename*=UTF-8''${name}`)
                res.setHeader('Content-Length', String(body.length))
                res.end(body)
            })
        },
    }
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
        mockFileManagerDownloads(),
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
