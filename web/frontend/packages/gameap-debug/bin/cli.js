#!/usr/bin/env node

import { spawn } from 'child_process';
import { dirname, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const packageRoot = resolve(__dirname, '..');

// PLUGIN_PATH was renamed to PLUGINS_PATH; the old name keeps working for one release.
const pluginsPath = process.env.PLUGINS_PATH ?? process.env.PLUGIN_PATH

if (!process.env.PLUGINS_PATH && process.env.PLUGIN_PATH) {
    console.warn('PLUGIN_PATH is deprecated and will be removed in a future release, use PLUGINS_PATH')
}

// Run vite dev server from the package directory
const vite = spawn('npx', ['vite'], {
    cwd: packageRoot,
    stdio: 'inherit',
    shell: true,
    env: {
        ...process.env,
        // Pass through PLUGINS_PATH, resolve relative paths from CWD
        PLUGINS_PATH: pluginsPath
            ? resolve(process.cwd(), pluginsPath)
            : undefined
    }
});

vite.on('close', (code) => {
    process.exit(code);
});
