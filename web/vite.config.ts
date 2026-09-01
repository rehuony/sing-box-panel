import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

import { configurationSchemaPlugin } from './vite/configuration-schema-plugin.ts';

export default defineConfig({
  base: './',
  publicDir: 'public',
  plugins: [react(), tailwindcss(), configurationSchemaPlugin()],
  resolve: {
    tsconfigPaths: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/tests/setup-tests.ts'],
    clearMocks: true,
  },
});
