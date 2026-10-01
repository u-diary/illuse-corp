/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** サーバー B のベース URL（末尾スラッシュなし） */
  readonly VITE_API_BASE_URL: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
