export type AppStage = 'preview' | 'test' | 'prod';

const raw = import.meta.env.VITE_APP_STAGE;

/**
 * Build-time only. Set via VITE_APP_STAGE in frontend/.env.development,
 * .env.test, and .env.production. This is not a user-facing setting.
 */
export const appStage: AppStage = raw === 'test' || raw === 'preview' ? raw : 'prod';

export const stageBadge: string | null = appStage === 'preview' ? 'PREVIEW BUILD' : appStage === 'test' ? 'TEST BUILD' : null;

export const workspaceStorageKey = `quarro.${appStage}.workspace.v3`;
