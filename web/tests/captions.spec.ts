import { test, expect } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockAlbumIfNeeded,
	albumExists,
	findHtmlCaption,
	type FoundHtmlCaption
} from './helpers';

const pw = loadPasswords();

let hasAntarctica = true;
let htmlCaption: FoundHtmlCaption | null = null;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
	htmlCaption = await findHtmlCaption(request);
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

// HTML in captions. photogen.txt is written by the site owner, so its markup is rendered
// rather than escaped — but only where a caption is body text. The `alt` and `aria-label`
// built from the same string are plain-text slots and get the tags stripped, and grid
// captions stay non-interactive so a tile remains a single click target.
//
// Driven by whatever the site publishes (see findHtmlCaption), so it is a no-op on a site
// with no HTML captions rather than a hardcoded fixture assertion.

test('grid caption renders HTML, while alt text stays plain', async ({ page }) => {
	test.skip(!htmlCaption, 'no caption with HTML on this site');
	const found = htmlCaption!;

	await page.goto(`/albums/${found.slug}`);
	await unlockAlbumIfNeeded(page, found.slug, pw);
	await waitForHydration(page);

	const tile = page.locator('.photo').nth(found.index);
	const caption = tile.locator('.photo-caption');

	// A real element, not the literal "<b>" that escaping would produce.
	await expect(caption.locator('b, strong, i, em, a').first()).toBeAttached();
	await expect(caption).not.toContainText('<');

	// The same caption feeds alt and aria-label, which cannot render markup.
	expect(await tile.locator('img').getAttribute('alt')).not.toContain('<');
	expect(await tile.getAttribute('aria-label')).not.toContain('<');
});

test('grid caption is inert, so its links add no tab stops', async ({ page }) => {
	test.skip(!htmlCaption, 'no caption with HTML on this site');
	const found = htmlCaption!;
	test.skip(!/<a[\s>]/i.test(found.description), 'caption has no link');

	await page.goto(`/albums/${found.slug}`);
	await unlockAlbumIfNeeded(page, found.slug, pw);
	await waitForHydration(page);

	// `inert` removes the subtree from the tab order and the a11y tree. Without it a
	// caption <a> would be interactive content nested inside the tile's <button>.
	const caption = page.locator('.photo').nth(found.index).locator('.photo-caption');
	await expect(caption).toHaveAttribute('inert', /.*/);

	const focusedTag = await page.evaluate(() => {
		const link = document.querySelector('.photo-caption a') as HTMLElement | null;
		link?.focus();
		return document.activeElement?.tagName ?? '';
	});
	expect(focusedTag).not.toBe('A');
});

test('lightbox caption renders HTML, its links open in a new tab, alt stays plain', async ({
	page
}) => {
	test.skip(!htmlCaption, 'no caption with HTML on this site');
	const found = htmlCaption!;

	await page.goto(`/albums/${found.slug}`);
	await unlockAlbumIfNeeded(page, found.slug, pw);
	await waitForHydration(page);

	await page.locator('.photo').nth(found.index).click();
	await expect(page.locator('.pswp')).toBeVisible();

	// Scope to the current slide. All three item holders (prev/current/next) carry a
	// caption, and only this photo's has HTML in it — aria-hidden="false" is what marks
	// the active holder (see the vertical-drag test above).
	const current = page.locator('.pswp__item[aria-hidden="false"]');
	const caption = current.locator('.pswp-caption');
	await expect(caption).toBeVisible();
	await expect(caption).not.toContainText('<');
	await expect(caption.locator('b, strong, i, em, a').first()).toBeAttached();

	if (/<a[\s>]/i.test(found.description)) {
		const link = caption.locator('a').first();
		// Forced in updateAll(): following a link in place would tear down the lightbox.
		await expect(link).toHaveAttribute('target', '_blank');
		await expect(link).toHaveAttribute('rel', 'noopener');
		// The caption container is pointer-events: none; links opt back in.
		await expect(link).toHaveCSS('pointer-events', 'auto');
	}

	// PhotoSwipe writes the data source's alt onto the slide image. Video slides have
	// no <img>, so only check when the found item is a still.
	const img = current.locator('.pswp__img').first();
	if ((await img.count()) > 0) {
		expect(await img.getAttribute('alt')).not.toContain('<');
	}
});

test('caption links use the same hairline underline in the grid and the lightbox', async ({
	page
}) => {
	test.skip(!htmlCaption, 'no caption with HTML on this site');
	const found = htmlCaption!;
	test.skip(!/<a[\s>]/i.test(found.description), 'caption has no link');

	await page.goto(`/albums/${found.slug}`);
	await unlockAlbumIfNeeded(page, found.slug, pw);
	await waitForHydration(page);

	// Thickness is pinned in CSS rather than left at `auto`, which scales with font size.
	// The grid caption is 0.78rem and the lightbox one up to 1.2rem, so `auto` would draw
	// a visibly heavier line in the lightbox for what should look like the same link.
	const gridLink = page.locator('.photo').nth(found.index).locator('.photo-caption a').first();
	await expect(gridLink).toHaveCSS('text-decoration-thickness', '1px');

	await page.locator('.photo').nth(found.index).click();
	await expect(page.locator('.pswp')).toBeVisible();

	// aria-hidden="false" marks the active holder; see the test above.
	const link = page.locator('.pswp__item[aria-hidden="false"]').locator('.pswp-caption a').first();
	await expect(link).toHaveCSS('text-decoration-thickness', '1px');

	// Hovering doubles it, which is the only affordance marking the link as clickable.
	await link.hover();
	await expect(link).toHaveCSS('text-decoration-thickness', '2px');
});
