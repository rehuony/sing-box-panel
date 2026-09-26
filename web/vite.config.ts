import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

import { configurationSchemaPlugin } from './plugins/configuration-schema.ts';

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
});
