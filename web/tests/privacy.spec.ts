import { test, expect } from '@playwright/test';
import { loadPasswords, unlockSiteIfNeeded } from './helpers';

const pw = loadPasswords();

test('privacy page loads and shows correct title', async ({ page }) => {
	const config = await page.request.get('/albums/config.json').then((r) => r.json());
	await page.goto('/privacy');
	await expect(page).toHaveTitle(`Privacy | ${config.siteName}`);
	await expect(page.locator('.card-site-name')).toContainText(config.siteName);
	await expect(page.locator('.card-title')).toContainText('Privacy');
});

test('privacy page always shows theme and site ID bullets', async ({ page }) => {
	await page.goto('/privacy');
	await expect(page.locator('li', { hasText: 'Theme' })).toBeVisible();
	await expect(page.locator('li', { hasText: 'Site ID' })).toBeVisible();
});

test('privacy page shows password bullets only when site is encrypted', async ({ page }) => {
	const config = await page.request.get('/albums/config.json').then((r) => r.json());
	await page.goto('/privacy');
	if (config.encrypted) {
		await expect(page.locator('li', { hasText: 'Passwords' })).toBeVisible();
		await expect(page.locator('li', { hasText: 'Album covers' })).toBeVisible();
	} else {
		await expect(page.locator('li', { hasText: 'Passwords' })).not.toBeVisible();
		await expect(page.locator('li', { hasText: 'Album covers' })).not.toBeVisible();
	}
});

test('privacy page has working back link and ?clear link', async ({ page }) => {
	await page.goto('/privacy');
	await expect(page.locator('a[href="/"]', { hasText: 'Back to albums' })).toBeVisible();
	await expect(page.locator('a[href="/?clear"]')).toBeVisible();
});

test('Back to albums from privacy restores scroll position', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.locator('.albums').waitFor();
	// Wait for all JS modules to finish loading so SvelteKit is fully hydrated.
	// Without this, dev mode (Vite serves many individual modules) may click the
	// privacy link before the SvelteKit router intercepts it, causing a full page
	// reload that resets module-level savedScrollY to 0.
	await page.waitForLoadState('networkidle');

	// Scroll to the bottom of the home page.
	await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
	const scrolledY = await page.evaluate(() => window.scrollY);

	// Skip if the page isn't tall enough to scroll (degenerate test config).
	if (scrolledY < 10) return;

	// Navigate to privacy via the footer link.
	await page.locator('a[href="/privacy"]').click();
	await expect(page).toHaveURL('/privacy');

	// Click Back to albums.
	await page.locator('a[href="/"]', { hasText: 'Back to albums' }).click();
	await page.locator('.albums').waitFor();

	// Scroll must be restored, not reset to the top.
	const restoredY = await page.evaluate(() => window.scrollY);
	expect(restoredY).toBeGreaterThan(10);
});
