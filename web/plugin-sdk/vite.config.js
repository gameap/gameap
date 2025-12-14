import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Rollup plugin that transforms ES module output to use global variables
 * instead of bare module specifiers (which browsers can't resolve).
 */
function globalExternalsPlugin() {
    const globals = {
        'vue': 'window.Vue',
        'vue-router': 'window.VueRouter',
        'pinia': 'window.Pinia',
        'axios': 'window.axios',
    };

    return {
        name: 'global-externals',
        renderChunk(code) {
            let result = code;
            for (const [moduleId, globalVar] of Object.entries(globals)) {
                // Handle named imports: import { ref, computed } from 'vue'
                const importRegex = new RegExp(
                    `import\\s*\\{([^}]+)\\}\\s*from\\s*["']${moduleId}["'];?`,
                    'g'
                );
                result = result.replace(importRegex, (_, imports) => {
                    const importList = imports.split(',').map(i => i.trim());
                    const destructure = importList.map(i => {
                        const parts = i.split(/\s+as\s+/);
                        if (parts.length === 2) {
                            return `${parts[0].trim()}: ${parts[1].trim()}`;
                        }
                        return i;
                    }).join(', ');
                    return `const { ${destructure} } = ${globalVar};`;
                });

                // Handle namespace imports: import * as Vue from 'vue'
                const importStarRegex = new RegExp(
                    `import\\s*\\*\\s*as\\s*(\\w+)\\s*from\\s*["']${moduleId}["'];?`,
                    'g'
                );
                result = result.replace(importStarRegex, (_, name) => {
                    return `const ${name} = ${globalVar};`;
                });

                // Handle default imports: import axios from 'axios'
                const importDefaultRegex = new RegExp(
                    `import\\s+(\\w+)\\s*from\\s*["']${moduleId}["'];?`,
                    'g'
                );
                result = result.replace(importDefaultRegex, (_, name) => {
                    return `const ${name} = ${globalVar};`;
                });
            }
            return { code: result, map: null };
        }
    };
}

/**
 * Base Vite configuration for GameAP plugins.
 * Plugin developers can extend this config in their own vite.config.js
 */
export function createPluginConfig(options = {}) {
    const {
        entry = 'src/index.ts',
        name = 'plugin',
        outDir = 'dist',
    } = options;

    return defineConfig({
        plugins: [vue()],
        build: {
            lib: {
                entry: resolve(process.cwd(), entry),
                formats: ['es'],
                fileName: () => `${name}.js`,
            },
            outDir,
            emptyOutDir: true,
            rollupOptions: {
                external: ['vue', 'vue-router', 'pinia', 'axios'],
                output: {
                    globals: {
                        vue: 'Vue',
                        'vue-router': 'VueRouter',
                        pinia: 'Pinia',
                        axios: 'axios',
                    },
                },
                plugins: [globalExternalsPlugin()],
            },
        },
        resolve: {
            alias: {
                '@': resolve(process.cwd(), 'src'),
            },
        },
    });
}

// SDK build configuration (exports types and context)
export default defineConfig({
    plugins: [vue()],
    build: {
        lib: {
            entry: resolve(__dirname, 'src/index.ts'),
            formats: ['es'],
            fileName: 'index',
        },
        outDir: 'dist',
        emptyOutDir: true,
        rollupOptions: {
            external: ['vue', 'vue-router', 'pinia'],
            output: {
                globals: {
                    vue: 'Vue',
                    'vue-router': 'VueRouter',
                    pinia: 'Pinia',
                },
            },
        },
    },
});
