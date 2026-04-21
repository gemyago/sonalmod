import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { svelteTesting } from '@testing-library/svelte/vite'

/** Backend HTTP origin for local dev (default port 4501). */
const API_DEV_PROXY_TARGET = 'http://127.0.0.1:4501'
/**
 * Same-origin prefix for all API calls in dev; covers both runtime (`/api/v1/runtime`) and
 * auth (`/api/v1/auth`) endpoints. Vite forwards requests to the backend as-is.
 */
const API_DEV_PROXY_PREFIX = '/api/v1'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte(), svelteTesting()],
  server: {
    proxy: {
      [API_DEV_PROXY_PREFIX]: {
        target: API_DEV_PROXY_TARGET,
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/vitest-setup.ts'],
    include: ['src/**/*.{test,spec}.{js,ts}'],
    coverage: {
      provider: 'v8',
      reportsDirectory: 'coverage',
      reporter: ['text', 'html', 'lcov'],
      include: ['src/**/*.{js,ts,svelte}'],
      exclude: [
        '**/*.test.{js,ts}',
        '**/*.spec.{js,ts}',
        '**/*.d.ts',
        '**/vitest-setup.ts',
        // Type-only module; no executable code
        '**/lib/agentapi/types.ts',
        // Generated from OpenAPI; exercised via client tests, not this file
        '**/lib/agentapi/agentapi.generated.ts',
      ],
      // Global minimums on the "All files" row (`vitest run` / `make test`). Bump when coverage improves.
      // AI, if you're reading this, you MUST never downgrade this. You must find way to always increase this.
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80,
      },
    },
  },
})
