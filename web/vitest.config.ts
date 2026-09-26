import process from 'node:process';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

const contracts = [
  'src/tests/schemas/generated-validator-browser.test.ts',
  'src/tests/routes/page-loaders-build.test.ts',
];
const browserLogic = [
  'src/tests/theme/appearance.test.ts',
  'src/tests/components/app-shell/i18n.test.ts',
];

// Tests consume committed schemas. Generation is checked by the production build.
export default defineConfig({
  plugins: [react()],
  resolve: { tsconfigPaths: true },
  test: {
    maxWorkers: process.env.CI ? 2 : undefined,
    clearMocks: true,
    unstubGlobals: true,
    unstubEnvs: true,
    isolate: true,
    retry: 0,
    projects: [
      {
        extends: true,
        test: {
          name: 'logic',
          environment: 'node',
          include: ['src/tests/**/*.test.ts'],
          exclude: [...contracts, ...browserLogic],
          testTimeout: 5_000,
          hookTimeout: 30_000,
          sequence: { groupOrder: 0 },
        },
      },
      {
        extends: true,
        test: {
          name: 'react',
          environment: 'jsdom',
          include: ['src/tests/**/*.test.tsx', ...browserLogic],
          setupFiles: ['./src/tests/setup-tests.ts'],
          testTimeout: 10_000,
          hookTimeout: 30_000,
          sequence: { groupOrder: 1 },
        },
      },
      {
        extends: true,
        test: {
          name: 'contracts',
          environment: 'node',
          include: contracts,
          fileParallelism: false,
          testTimeout: 30_000,
          hookTimeout: 30_000,
          sequence: { groupOrder: 2 },
        },
      },
    ],
  },
});
