import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['esm'],
  target: 'node22',
  outDir: 'dist',
  clean: true,
  sourcemap: true,
  splitting: false,
  bundle: true,
  // Do NOT bundle native modules — they must be loaded at runtime
  external: [
    'better-sqlite3',
    'sqlite-vec',
    'keytar',
  ],
  // Banner ensures the output is treated as an ESM CLI entrypoint
  banner: {
    js: `#!/usr/bin/env node
import { createRequire as cr } from 'module';
const require = cr(import.meta.url);`,
  },
  esbuildOptions(options) {
    options.platform = 'node';
  },
});
