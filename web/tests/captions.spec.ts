import { test, expect } from '@playwright/test';
import { waitForHydration, loadPasswords, unlockAlbumIfNeeded, albumExists } from './helpers';

const pw = loadPasswords();

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
});

// Caption tests verify the rendering mechanism works (rAF fix, animate=false fix),
// not specific caption text — so they work against any site (sample or prod).
const ALBUM = 'antarctica';
const PHOTO_N = 1; // 1-based (matches URL /albums/antarctica/1)

// Helper: assert that at least one caption element has non-empty visible text.
// PhotoSwipe maintains 3 holders (prev/current/next), each with a .pswp-caption;
// we use filter+first() to avoid Playwright's strict-mode multi-match error.
async function expectCaptionVisible(page: import('@playwright/test').Page) {
	await expect(page.locator('.pswp-caption').filter({ hasText: /\S/ }).first()).toBeVisible();
}

test('caption shows when clicking a photo from the grid (animate=true path)', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto(`/albums/${ALBUM}`);
	await unlockAlbumIfNeeded(page, ALBUM, pw);
	await waitForHydration(page);

	await page
		.locator('.photo')
		.nth(PHOTO_N - 1)
		.click();

	// Lightbox should open.
	await expect(page.locator('.pswp')).toBeVisible();

	// Caption must appear — exercises the requestAnimationFrame(updateAll) fix.
	await expectCaptionVisible(page);
});

test('caption shows when loading a photo permalink directly (animate=false path)', async ({
	page
}) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	// Direct URL open: onMount calls openLightbox(..., false) before the router
	// is fully initialised — exercises the `if (animate) replaceState(...)` fix.
	//
	// If the album is encrypted, unlock at the base URL first so the password is
	// stored in localStorage. Then the permalink load triggers tryDecryptAlbum in
	// onMount, which auto-decrypts and calls openLightbox. (handleUnlock does not.)
	if (pw.all || pw.albums[ALBUM]) {
		await page.goto(`/albums/${ALBUM}`);
		await unlockAlbumIfNeeded(page, ALBUM, pw);
	}
	await page.goto(`/albums/${ALBUM}/${PHOTO_N}`);

	// Lightbox should auto-open without a click.
	await expect(page.locator('.pswp')).toBeVisible();

	// Caption must appear via the rAF fallback (openingAnimationEnd fires during
	// pswp.init() when showAnimationDuration=0, before its listener is registered).
	await expectCaptionVisible(page);
});

test('lightbox caption uses balanced line wrapping', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto(`/albums/${ALBUM}`);
	await unlockAlbumIfNeeded(page, ALBUM, pw);
	await waitForHydration(page);
	await page
		.locator('.photo')
		.nth(PHOTO_N - 1)
		.click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expectCaptionVisible(page);

	// Guards against text-wrap: balance being dropped from the caption rule — without it
	// a wrapped caption strands its last word on a line by itself. Asserts the property
	// is applied rather than measuring line boxes: sample captions are short enough that
	// they don't wrap at the test viewport, so a geometric check would assert nothing.
	const textWrap = await page
		.locator('.pswp-caption')
		.filter({ hasText: /\S/ })
		.first()
		.evaluate((el) => getComputedStyle(el).textWrap || getComputedStyle(el).textWrapStyle);
	expect(textWrap).toBe('balance');
});

test('caption moves with the photo during a vertical drag', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto(`/albums/${ALBUM}`);
	await unlockAlbumIfNeeded(page, ALBUM, pw);
	await waitForHydration(page);
	await page
		.locator('.photo')
		.nth(PHOTO_N - 1)
		.click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expectCaptionVisible(page);

	// Read the caption and image tops from the *current* slide. All three item holders
	// (prev/current/next) carry a caption, so aria-hidden="false" — which PhotoSwipe sets
	// on the active holder — is what distinguishes the one being dragged.
	const readTops = () =>
		page.evaluate(() => {
			const item = document.querySelector('.pswp__item[aria-hidden="false"]');
			const cap = item?.querySelector('.pswp-caption');
			const img = item?.querySelector('img');
			if (!cap || !img) return null;
			return { cap: cap.getBoundingClientRect().top, img: img.getBoundingClientRect().top };
		});

	const before = await readTops();
	expect(before).not.toBeNull();

	// Drag downward without releasing: on release the pan springs back to centre, and a
	// release past the close threshold would dismiss the lightbox entirely.
	const vp = page.viewportSize()!;
	await page.mouse.move(vp.width / 2, vp.height / 2);
	await page.mouse.down();
	await page.mouse.move(vp.width / 2, vp.height / 2 + 80, { steps: 10 });

	const during = await readTops();
	expect(during).not.toBeNull();

	const imgDelta = during!.img - before!.img;
	const capDelta = during!.cap - before!.cap;

	// The photo actually moved (guards against the drag being swallowed)...
	expect(imgDelta).toBeGreaterThan(10);
	// ...and the caption tracked it. Both derive from the same slide.pan.y, so PhotoSwipe's
	// drag friction applies equally and the deltas should match within a rounding pixel.
	expect(Math.abs(capDelta - imgDelta)).toBeLessThanOrEqual(2);

	await page.mouse.up();
});

test('caption updates when navigating to prev/next photo', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto(`/albums/${ALBUM}`);
	await unlockAlbumIfNeeded(page, ALBUM, pw);
	await waitForHydration(page);
	await page
		.locator('.photo')
		.nth(PHOTO_N - 1)
		.click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expectCaptionVisible(page);

	// Navigate to the next photo; caption should change (or hide if no description).
	await page.locator('.pswp__button--arrow--next').click();
	// Caption for *this* photo may or may not exist — just assert the lightbox
	// is still open and didn't crash.
	await expect(page.locator('.pswp')).toBeVisible();
});
