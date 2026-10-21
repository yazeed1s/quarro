import { existsSync } from 'node:fs';
import { defineConfig, loadEnv } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import wails from '@wailsio/runtime/plugins/vite';

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
    const stage = loadEnv(mode, process.cwd(), 'VITE_APP_STAGE').VITE_APP_STAGE || 'prod';
    if (!['preview', 'test', 'prod'].includes(stage)) throw new Error(`Invalid VITE_APP_STAGE: ${stage}`);
    return {
        server: {
            host: '127.0.0.1',
            port: Number(process.env.WAILS_VITE_PORT) || 9245,
            strictPort: true,
            // The brand marks are imported from ../brand, the single source
            // brand/generate_icons.py writes to.
            fs: { allow: ['.', '../brand'] },
        },
        // The UI preview has no native event dependency. Enable typed native events
        // after Wails generates the bindings for a desktop build.
        plugins: [
            svelte(),
            {
                name: 'quarro-stage',
                generateBundle() {
                    this.emitFile({
                        type: 'asset',
                        fileName: 'app-stage.json',
                        source: JSON.stringify(stage),
                    });
                },
            },
            ...(existsSync('./bindings/github.com/wailsapp/wails/v3/internal/eventcreate') ? [wails('./bindings')] : []),
        ],
    };
});
