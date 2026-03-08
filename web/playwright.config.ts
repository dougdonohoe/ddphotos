import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: './tests',
	// No webServer — tests run against the Docker Apache container.
	// deploy-photos.sh uses port 8080; make photos-playwright-test uses 8081 to avoid
	// conflicts. Override via PLAYWRIGHT_BASE_URL env var if needed.
	use: {
		baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:8080',
	},
	projects: [
		{ name: 'chromium', use: { ...devices['Desktop Chrome'] } },
	],
});
