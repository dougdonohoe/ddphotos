import { test, expect } from '@playwright/test';

test('privacy page loads and shows correct title', async ({ page }) => {
	const config = await page.request.get('/albums/config.json').then((r) => r.json());
	await page.goto('/privacy');
	await expect(page).toHaveTitle(`Privacy | ${config.siteName}`);
	await expect(page.locator('h1')).toContainText(config.siteName);
	await expect(page.locator('h1')).toContainText('Privacy');
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
