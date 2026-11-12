import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { defineConfig } from 'vitest/config';

// Component tests need Solid's JSX transform. Load vite-plugin-solid through @wxt-dev/module-solid (which already
// depends on it) so the build and the tests use the same version and no extra dependency is pinned.
const fromModule = createRequire(createRequire(import.meta.url).resolve('@wxt-dev/module-solid'));
const solidPath = fromModule.resolve('vite-plugin-solid');
const { default: solid } = await import(pathToFileURL(solidPath).href);

export default defineConfig({
  plugins: [solid()],
  resolve: { conditions: ['browser', 'development'] },
});
