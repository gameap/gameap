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
        rollupOptions: {
            input: {
                main: resolve(__dirname, 'index.html')
            },
            output: {
                manualChunks: (id) => {
                    // Manual chunking disabled due to circular dependency issues
                    // Vite's automatic chunking will handle code splitting
                    if (id.includes('/views/adminviews/')) {
                        return 'admin-views';
                    }
                    if (id.includes('/filemanager/')) {
                        return 'filemanager';
                    }
                }
            }
        }
    },
    server: {
        proxy: {
            '/api': {
                target: 'http://localhost:8000',
                changeOrigin: true,
            },
        },
    },
});