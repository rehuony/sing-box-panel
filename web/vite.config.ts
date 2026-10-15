import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  server: { fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), fileURLToPath(new URL('../api', import.meta.url))] } },
  base: './',
  publicDir: 'public',
  plugins: [react(), tailwindcss()],
  resolve: {
    tsconfigPaths: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
