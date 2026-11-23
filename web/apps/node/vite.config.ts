import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

// During development, the console API of a node running on this host answers
// /api; the proxy keeps the browser's loopback Host the console requires.
export default defineConfig({
  server: {
    fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), fileURLToPath(new URL('../../packages/shared/src', import.meta.url)), fileURLToPath(new URL('../../node_modules', import.meta.url))] },
    proxy: { '/api': { target: 'http://127.0.0.1:3001' } },
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
