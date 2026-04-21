/// <reference types="svelte" />
/// <reference types="vite/client" />

/**
 * Client bundle env: only `VITE_*` keys from `.env` / CI are exposed. Declare every key
 * read via `import.meta.env` in app code here so TypeScript matches Vite’s behavior.
 */
interface ImportMetaEnv {
  /** Shown on Home; set via `.env` / `.env.*` (see `.env.example`). */
  readonly VITE_APP_TITLE?: string
  /** Agent API base (origin or same-origin path). Defaults in app to `/api/v1/runtime`. */
  readonly VITE_AGENT_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
