/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** サーバー B（onboard）のベース URL（末尾スラッシュなし） */
  readonly VITE_ONBOARD_API_BASE_URL: string;
  /** サーバー C（fire）のベース URL（末尾スラッシュなし） */
  readonly VITE_FIRE_API_BASE_URL: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
