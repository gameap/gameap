import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

export default {
    content: [
        resolve(__dirname, 'index.html'),
        resolve(__dirname, 'js/**/*.{js,ts,jsx,tsx,vue}'),
        resolve(__dirname, 'packages/**/*.{js,vue,css}'),
    ],
    theme: {
        extend: {},
    },
    plugins: [
        require('@tailwindcss/aspect-ratio'),
    ],
    darkMode: 'selector',
}