import { createPluginConfig } from '@gameap/plugin-sdk/vite';

export default createPluginConfig({
    entry: 'src/index.ts',
    name: 'plugin',
    outDir: 'dist',
});
