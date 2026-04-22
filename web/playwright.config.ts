import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: './tests',
	// No webServer — bin/run-tests.sh sets PLAYWRIGHT_BASE_URL to the appropriate port
	// (5174 dev, 8083 Apache, 8084 nginx). Default 8080 is used by deploy-photos.sh.
	timeout: 15_000,      // per-test timeout (default is 30s; 15s surfaces failures faster)
	reporter: 'list',     // print each result as it completes rather than buffering to the end
	use: {
		baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:8080',
		actionTimeout: 10_000, // per-action timeout (locator waits, clicks, etc.)
	},
	expect: {
		timeout: 10_000,   // per-assertion retry timeout (default is 5s; 10s needed for
		                   // Apache mode where {#if browser} components appear after hydration)
	},
	projects: [
		{ name: 'chromium', use: { ...devices['Desktop Chrome'] } },
	],
});
