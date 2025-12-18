import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const frontendRoot = resolve(__dirname, '../..')

export default defineConfig({
  plugins: [vue()],
  publicDir: resolve(frontendRoot, 'public'),
  build: {
    lib: {
      entry: resolve(__dirname, 'index.js'),
      formats: ['es'],
      fileName: () => 'index.js'
    },
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      external: [
        'vue',
        'vue-router',
        'pinia',
        'naive-ui',
        'axios',
        '@gameap/plugin-sdk',
        // Externalize static assets - they're served by the host application
        /^\/images\/.*/,
        /^\/fonts\/.*/,
      ],
      output: {
        globals: {
          vue: 'Vue',
          'vue-router': 'VueRouter',
          pinia: 'Pinia',
          'naive-ui': 'naive',
          axios: 'axios'
        }
      }
    }
  },
  resolve: {
    alias: {
      '@': resolve(frontendRoot, 'js'),
      '@gameap/ui': resolve(frontendRoot, 'packages/gameap-ui')
    }
  },
  css: {
    postcss: resolve(frontendRoot, 'postcss.config.js'),
    preprocessorOptions: {
      scss: {
        api: 'modern-compiler',
        silenceDeprecations: ['import']
      }
    }
  }
})
