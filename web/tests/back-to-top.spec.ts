import { test, expect } from '@playwright/test';

const MOBILE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 800 };

// Scroll past the 600px threshold and fire the scroll event.
// Everything runs in one evaluate call so the layout reflow from min-height is applied
// synchronously (via getBoundingClientRect) before scrollTo reads the page dimensions.
async function simulateScroll(page: any, y = 700) {
	await page.evaluate((scrollY: number) => {
		document.body.style.minHeight = (scrollY + 1000) + 'px';
		document.body.getBoundingClientRect(); // force synchronous layout
		window.scrollTo(0, scrollY);
		window.dispatchEvent(new Event('scroll'));
	}, y);
}

// Album grid page: back-to-top should appear on both desktop and mobile
test('album page: back-to-top appears after scrolling on desktop', async ({ page }) => {
	await page.setViewportSize(DESKTOP);
	await page.goto('/albums/antarctica');
	// .layout-ready is set in the album page's onMount — guarantees hydration is complete
	await page.locator('.gallery.layout-ready').waitFor({ state: 'attached' });
	await simulateScroll(page);
	await expect(page.locator('.back-to-top')).toBeVisible();
});

test('album page: back-to-top appears after scrolling on mobile', async ({ page }) => {
	await page.setViewportSize(MOBILE);
	await page.goto('/albums/antarctica');
	await page.locator('.gallery.layout-ready').waitFor({ state: 'attached' });
	await simulateScroll(page);
	await expect(page.locator('.back-to-top')).toBeVisible();
});

// Home page: back-to-top should appear on mobile only.
// Uses toPass to retry scroll+check in case the scroll listener isn't registered yet
// (can race against other parallel tests in the dev server).
test('home page: back-to-top appears after scrolling on mobile', async ({ page }) => {
	await page.setViewportSize(MOBILE);
	await page.goto('/');
	await expect(async () => {
		await simulateScroll(page);
		await expect(page.locator('.back-to-top')).toBeVisible({ timeout: 1000 });
	}).toPass({ timeout: 8000 });
});

test('home page: back-to-top is hidden on desktop', async ({ page }) => {
	await page.setViewportSize(DESKTOP);
	await page.goto('/');
	await expect(async () => {
		await simulateScroll(page);
		await expect(page.locator('.back-to-top')).toBeHidden({ timeout: 1000 });
	}).toPass({ timeout: 8000 });
});
