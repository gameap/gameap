import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue'
import { viteCommonjs } from '@originjs/vite-plugin-commonjs'
import { resolve } from 'path'

export default defineConfig({
    plugins: [
        viteCommonjs(),
        vue()
    ],
    base: '/',
    publicDir: 'public',
    resolve: {
        alias: {
            '@': resolve(__dirname, 'js'),
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
                experimentalMinChunkSize: 5000, // 5KB minimum chunk size
            }
        },
        cssCodeSplit: true
    },
    server: {
        proxy: {
            '/lang': {
                target: 'http://localhost:8025',
                changeOrigin: true,
            },
            '/api': {
                target: 'http://localhost:8025',
                changeOrigin: true,
            },
        },
    },
});