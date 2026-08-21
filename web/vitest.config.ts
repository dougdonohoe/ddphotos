import { defineConfig } from 'vitest/config';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Unit tests for plain TypeScript helpers in src/lib. Deliberately separate from
// vite.config.ts, which vitest would otherwise pick up: that config loads config/defaults.env,
// resolves the album directory and installs the SvelteKit plugin, none of which a pure
// function needs.
//
// `include` is narrowed to src for the same reason it cannot be left at the default: the
// default pattern matches *.spec.ts too, which would hand vitest the Playwright suite in
// tests/ and fail on the browser fixtures. Unit tests live beside the code they cover;
// tests/ stays Playwright-only.
export default defineConfig({
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'node'
	},
	resolve: {
		alias: { $lib: resolve(__dirname, 'src/lib') }
	}
});
