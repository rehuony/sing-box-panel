import process from 'node:process';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  server: { fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), fileURLToPath(new URL('../../packages/shared/src', import.meta.url))] } },
  plugins: [react()],
  resolve: { tsconfigPaths: true },
  test: {
    maxWorkers: process.env.CI ? 2 : undefined,
    clearMocks: true,
    unstubGlobals: true,
    environment: 'jsdom',
    include: ['src/tests/**/*.test.tsx'],
    setupFiles: ['./src/tests/setup-tests.ts'],
    testTimeout: 10_000,
  },
});
