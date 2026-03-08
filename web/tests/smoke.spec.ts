import { test, expect } from '@playwright/test';

// Smoke tests — basic rendering checks that catch build/deploy regressions.
// Albums checked here (Antarctica, Uganda) are present in both sample and prod.

test('home page lists albums including known overlap albums', async ({ page }) => {
	await page.goto('/');
	// Spot-check albums present in both sample and prod
	await expect(page.locator('.album-card', { hasText: 'Antarctica' })).toBeVisible();
	await expect(page.locator('.album-card', { hasText: 'Uganda' })).toBeVisible();
});

test('home page album cards show description', async ({ page }) => {
	await page.goto('/');
	// Antarctica's description is stable across sample and prod
	await expect(page.locator('.album-card', { hasText: 'bottom of the world' })).toBeVisible();
});

test('album page shows title, description, and photo count', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await expect(page.locator('h1')).toHaveText('Antarctica');
	await expect(page.locator('.description')).toContainText('bottom of the world');
	await expect(page.locator('.meta')).toContainText(/\d+ photos/);
});

test('album page renders photos in the grid', async ({ page }) => {
	await page.goto('/albums/antarctica');
	// At least one photo button should be present
	await expect(page.locator('.photo').first()).toBeVisible();
});

test('album page has correct Open Graph tags', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', 'Antarctica');
	await expect(page.locator('meta[property="og:type"]')).toHaveAttribute('content', 'website');
	// og:image must be a JPEG (not WebP) — iMessage and many crawlers don't support WebP previews
	await expect(page.locator('meta[property="og:image"]')).toHaveAttribute('content', /antarctica\/cover\.jpg$/);
});

test('home page has Open Graph image tag pointing to a JPEG', async ({ page }) => {
	await page.goto('/');
	// Must be a JPEG (cover.jpg) for iMessage / crawler compatibility — not WebP
	await expect(page.locator('meta[property="og:image"]')).toHaveAttribute('content', /\/cover\.jpg$/);
});
