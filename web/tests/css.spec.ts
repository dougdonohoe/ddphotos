import { test, expect } from '@playwright/test';
import { siteCustomCss } from './helpers';

// Custom CSS tests — verify that a custom CSS file is applied site-wide when
// configured via the -css flag.
//
// Tests 1 and 2 derive CSS presence from config.json (programmatic).
// Tests 3 and 4 require PLAYWRIGHT_CUSTOM_CSS to be set, since they check
// specific values only meaningful for the known sample custom.css file.

const hasCustomCss = !!process.env.PLAYWRIGHT_CUSTOM_CSS;

test('custom CSS <link> is injected into the page', async ({ page, request }) => {
	const css = await siteCustomCss(request);
	test.skip(!css, 'no custom CSS configured');
	await page.goto('/');
	const link = page.locator(`link[rel="stylesheet"][href*="${css}"]`);
	await expect(link).toHaveCount(1);
});

test('custom CSS <link> is NOT present when CSS is not configured', async ({ page, request }) => {
	const css = await siteCustomCss(request);
	test.skip(!!css, 'CSS is configured — skipping no-CSS check');
	await page.goto('/');
	const link = page.locator('link[rel="stylesheet"][href*="custom.css"]');
	await expect(link).toHaveCount(0);
});

test('custom CSS overrides --text-color-2nd on home page', async ({ page }) => {
	test.skip(!hasCustomCss, 'no custom CSS configured');
	await page.goto('/');
	// The custom.css sets --text-color-2nd to teal (#2a9d8f).
	// Verify via a computed style on an element that uses this variable.
	// The site title <h1> uses --text-color-2nd for its color.
	const h1 = page.locator('h1');
	await expect(h1).toBeVisible();
	const color = await h1.evaluate((el) =>
		getComputedStyle(el).getPropertyValue('--text-color-2nd').trim()
	);
	expect(color).toBe('#2a9d8f');
});

test('custom CSS applies border-radius to album cards', async ({ page }) => {
	test.skip(!hasCustomCss, 'no custom CSS configured');
	await page.goto('/');
	const card = page.locator('.album-card').first();
	await expect(card).toBeVisible();
	const radius = await card.evaluate((el) => getComputedStyle(el).borderRadius);
	expect(radius).toBe('16px');
});
