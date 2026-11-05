import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

// During development, a panel running on this host at the root base path
// answers /api; the proxy keeps the browser's Host, so the panel's same-origin
// checks accept the development server's origin.
export default defineConfig({
  server: {
    fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), fileURLToPath(new URL('../../packages/shared/src', import.meta.url)), fileURLToPath(new URL('../../node_modules', import.meta.url)), fileURLToPath(new URL('../../../internal/configuration', import.meta.url)), fileURLToPath(new URL('../../../internal/sing-box-panel/subscription/templates', import.meta.url))] },
    proxy: { '/api': { target: 'http://127.0.0.1:3000' } },
  },
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
