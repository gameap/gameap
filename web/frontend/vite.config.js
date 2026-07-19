import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue'
import { viteCommonjs } from '@originjs/vite-plugin-commonjs'
import { resolve } from 'path'
import { closeSync, openSync } from 'fs'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

const keepGitkeep = () => ({
    name: 'keep-gitkeep',
    closeBundle() {
        closeSync(openSync(resolve(__dirname, '../static/dist/.gitkeep'), 'w'))
    },
})

export default defineConfig({
    plugins: [
        viteCommonjs(),
        vue(),
        Components({
            resolvers: [NaiveUiResolver()],
            dts: false,
        }),
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