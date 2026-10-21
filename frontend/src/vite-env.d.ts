/// <reference types="svelte" />
/// <reference types="vite/client" />

interface ImportMetaEnv {
    readonly VITE_APP_STAGE?: 'preview' | 'test' | 'prod';
}

interface ImportMeta {
    readonly env: ImportMetaEnv;
}
