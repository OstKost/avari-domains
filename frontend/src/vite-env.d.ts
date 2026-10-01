/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_YM_ID?: string;
  readonly VITE_YM_WEBVISOR?: string;
  readonly VITE_GA_ID?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
