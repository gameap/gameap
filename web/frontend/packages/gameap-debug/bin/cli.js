#!/usr/bin/env node

import { spawn } from 'child_process';
import { dirname, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const packageRoot = resolve(__dirname, '..');

// Run vite dev server from the package directory
const vite = spawn('npx', ['vite'], {
    cwd: packageRoot,
    stdio: 'inherit',
    shell: true,
    env: {
        ...process.env,
        // Pass through PLUGIN_PATH, resolve relative paths from CWD
        PLUGIN_PATH: process.env.PLUGIN_PATH
            ? resolve(process.cwd(), process.env.PLUGIN_PATH)
            : undefined
    }
});

vite.on('close', (code) => {
    process.exit(code);
});
