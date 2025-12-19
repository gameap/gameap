import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

export default {
    content: [
        resolve(__dirname, 'index.html'),
        resolve(__dirname, 'js/**/*.{js,ts,jsx,tsx,vue}'),
        resolve(__dirname, 'packages/gameap-ui/**/*.{js,vue}'),
        resolve(__dirname, 'packages/gameap-debug/src/**/*.{js,vue}'),
        resolve(__dirname, 'packages/gameap-frontend/*.{js,vue}'),
    ],
    theme: {
        extend: {},
    },
    plugins: [
        require('@tailwindcss/aspect-ratio'),
    ],
    darkMode: 'selector',
}