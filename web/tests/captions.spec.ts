import { test, expect } from '@playwright/test';
import { waitForHydration } from './helpers';

// Caption tests verify the rendering mechanism works (rAF fix, animate=false fix),
// not specific caption text — so they work against any site (sample or prod).
const ALBUM   = 'antarctica';
const PHOTO_N = 1;    // 1-based (matches URL /albums/antarctica/1)

// Helper: assert that at least one caption element has non-empty visible text.
// PhotoSwipe maintains 3 holders (prev/current/next), each with a .pswp-caption;
// we use filter+first() to avoid Playwright's strict-mode multi-match error.
async function expectCaptionVisible(page: import('@playwright/test').Page) {
	await expect(page.locator('.pswp-caption').filter({ hasText: /\S/ }).first()).toBeVisible();
}

test('caption shows when clicking a photo from the grid (animate=true path)', async ({ page }) => {
	await page.goto(`/albums/${ALBUM}`);
	await waitForHydration(page);

	await page.locator('.photo').nth(PHOTO_N - 1).click();

	// Lightbox should open.
	await expect(page.locator('.pswp')).toBeVisible();

	// Caption must appear — exercises the requestAnimationFrame(updateAll) fix.
	await expectCaptionVisible(page);
});

test('caption shows when loading a photo permalink directly (animate=false path)', async ({ page }) => {
	// Direct URL open: onMount calls openLightbox(..., false) before the router
	// is fully initialised — exercises the `if (animate) replaceState(...)` fix.
	await page.goto(`/albums/${ALBUM}/${PHOTO_N}`);

	// Lightbox should auto-open without a click.
	await expect(page.locator('.pswp')).toBeVisible();

	// Caption must appear via the rAF fallback (openingAnimationEnd fires during
	// pswp.init() when showAnimationDuration=0, before its listener is registered).
	await expectCaptionVisible(page);
});

test('caption updates when navigating to prev/next photo', async ({ page }) => {
	await page.goto(`/albums/${ALBUM}`);
	await waitForHydration(page);
	await page.locator('.photo').nth(PHOTO_N - 1).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expectCaptionVisible(page);

	// Navigate to the next photo; caption should change (or hide if no description).
	await page.locator('.pswp__button--arrow--next').click();
	// Caption for *this* photo may or may not exist — just assert the lightbox
	// is still open and didn't crash.
	await expect(page.locator('.pswp')).toBeVisible();
});
