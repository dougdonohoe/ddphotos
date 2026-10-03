import { test, expect, type Page } from '@playwright/test';
import { waitForHydration, loadPasswords, unlockAlbumIfNeeded, albumExists } from './helpers';

// Lightbox click behavior on photos that can and cannot zoom.
//
// PhotoSwipe allows zoom only when a photo is shown below the size it is given, which is the
// source's width and height from index.json. Antarctica's largest sample source is 2592x2160,
// so a larger viewport makes every photo in it non-zoomable, and the default viewport
// makes them zoomable.

const pw = loadPasswords();
const slug = 'antarctica';

const currentImg = (page: Page) =>
	page.locator('.pswp__item[aria-hidden="false"] .pswp__img').last();

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, slug);
});

/** Opens the lightbox on the album's first photo (not video) tile. */
async function openFirstPhoto(page: Page) {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto(`/albums/${slug}`);
	await unlockAlbumIfNeeded(page, slug, pw);
	await waitForHydration(page);
	const tile = page.locator('.photo:not(:has(.video-badge))').first();
	await tile.click();
	await expect(page.locator('.pswp--open')).toBeVisible();
	await expect(currentImg(page)).toBeVisible();
}

test.describe('photo that fits at full size', () => {
	test.use({ viewport: { width: 2700, height: 2300 } });

	test('shows a plain cursor and stays open when clicked', async ({ page }) => {
		await openFirstPhoto(page);
		await expect(page.locator('.pswp')).not.toHaveClass(/pswp--zoom-allowed/);
		await expect(currentImg(page)).toHaveCSS('cursor', 'default');

		await currentImg(page).click();
		// A close would come late: PhotoSwipe holds a click ~300ms to rule out a double tap,
		// then animates for ~333ms before removing .pswp, so wait well past both.
		await page.waitForTimeout(1500);
		await expect(page.locator('.pswp--open')).toBeVisible();
	});
});

test('photo shown below full size zooms on click', async ({ page }) => {
	await openFirstPhoto(page);
	await expect(page.locator('.pswp')).toHaveClass(/pswp--zoom-allowed/);
	await expect(currentImg(page)).toHaveCSS('cursor', 'zoom-in');

	await currentImg(page).click();
	await expect(page.locator('.pswp')).toHaveClass(/pswp--zoomed-in/);
	await expect(page.locator('.pswp--open')).toBeVisible();
});
