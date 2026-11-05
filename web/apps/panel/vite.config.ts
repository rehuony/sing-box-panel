import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  server: { fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), fileURLToPath(new URL('../../packages/shared/src', import.meta.url)), fileURLToPath(new URL('../../node_modules', import.meta.url)), fileURLToPath(new URL('../../../internal/configuration', import.meta.url)), fileURLToPath(new URL('../../../internal/sing-box-panel/subscription/templates', import.meta.url))] } },
  base: './',
  publicDir: '../../packages/shared/public',
  plugins: [react(), tailwindcss()],
  resolve: {
    tsconfigPaths: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
