export default {
    content: [
        "./index.html",
        "./js/**/*.{js,ts,jsx,tsx,vue}",
    ],
    theme: {
        extend: {},
    },
    plugins: [
        require('@tailwindcss/aspect-ratio'),
    ],
    darkMode: 'selector',
}