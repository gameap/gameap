const tailwindcss = require('tailwindcss');
const postcssPresetEnv = require('postcss-preset-env');
const path = require('path');

module.exports = {
  plugins: [
    postcssPresetEnv,
    tailwindcss(path.resolve(__dirname, 'tailwind.config.js'))
  ],
};