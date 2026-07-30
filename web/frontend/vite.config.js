import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue'
import { viteCommonjs } from '@originjs/vite-plugin-commonjs'
import { resolve } from 'path'
import { closeSync, openSync, readFileSync } from 'fs'
import { createHash } from 'crypto'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

const keepGitkeep = () => ({
    name: 'keep-gitkeep',
    closeBundle() {
        closeSync(openSync(resolve(__dirname, '../static/dist/.gitkeep'), 'w'))
    },
})

// Ships packages/gameap-ui/theme.css verbatim as /theme.css (bypassing
// PostCSS/minification) and links it before the app styles. The stable,
// unhashed name is the public theming contract for plugins and operators.
const themeCss = () => {
    const themePath = resolve(__dirname, 'packages/gameap-ui/theme.css')
    const version = () =>
        createHash('sha256').update(readFileSync(themePath)).digest('hex').slice(0, 8)

    return {
        name: 'gameap-theme-css',
        configureServer(server) {
            server.watcher.add(themePath)
            server.watcher.on('change', (file) => {
                if (file === themePath) {
                    server.ws.send({ type: 'full-reload' })
                }
            })
            server.middlewares.use('/theme.css', (req, res) => {
                res.setHeader('Content-Type', 'text/css; charset=utf-8')
                res.setHeader('Cache-Control', 'no-store')
                res.end(readFileSync(themePath))
            })
        },
        generateBundle() {
            this.emitFile({
                type: 'asset',
                fileName: 'theme.css',
                source: readFileSync(themePath, 'utf-8'),
            })
        },
        transformIndexHtml() {
            return [{
                tag: 'link',
                attrs: { rel: 'stylesheet', href: `/theme.css?v=${version()}` },
                injectTo: 'head-prepend',
            }]
        },
    }
}

export default defineConfig({
    plugins: [
        viteCommonjs(),
        vue(),
        Components({
            resolvers: [NaiveUiResolver()],
            dts: false,
        }),
        themeCss(),
        keepGitkeep(),
    ],
    base: '/',
    publicDir: 'public',
    resolve: {
        alias: {
            '@': resolve(__dirname, 'js'),
            '@gameap/ui': resolve(__dirname, 'packages/gameap-ui'),
        },
    },
    build: {
        outDir: '../static/dist',
        emptyOutDir: true,
        chunkSizeWarningLimit: 500,
        modulePreload: {
            // Disable modulePreload for large chunks that should be truly lazy-loaded
            resolveDependencies: (filename, deps, { hostId, hostType }) => {
                // Don't preload filemanager chunk - it should load only when Files tab is clicked
                return deps.filter(dep => !dep.includes('filemanager'))
            }
        },
        rollupOptions: {
            input: {
                main: resolve(__dirname, 'index.html')
            },
            output: {
                // Merge small chunks to reduce HTTP requests
                experimentalMinChunkSize: 20000, // 20KB minimum chunk size
            }
        },
        cssCodeSplit: true
    },
    server: {
        proxy: {
            '/lang': {
                target: 'http://127.0.0.1:8025',
                changeOrigin: true,
            },
            '/api': {
                target: 'http://127.0.0.1:8025',
                changeOrigin: true,
                ws: true,
            },
            '/plugins.css': {
                target: 'http://127.0.0.1:8025',
                changeOrigin: true,
            },
            '/plugins.js': {
                target: 'http://127.0.0.1:8025',
                changeOrigin: true,
            },
        },
    },
});