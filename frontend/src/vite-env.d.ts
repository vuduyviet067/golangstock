/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** "true" shows self-registration on the login screen; any other value or unset hides it. */
  readonly VITE_ALLOW_REGISTRATION?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
