import { test, expect, type Page } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockSiteIfNeeded,
	unlockAlbumIfNeeded,
	slugFromCard,
	findHtmlCaption,
	type FoundHtmlCaption
} from './helpers';

// The lightbox's caption toggle: a top-bar button, right of zoom, that hides and shows
// captions for the rest of the browser session. Present only in albums with captions.
//
// Albums are told apart by the grid's own caption overlays rather than index.json, so this
// works on password-protected variants too, where index.json is encrypted.

const pw = loadPasswords();

const toggle = (page: Page) => page.locator('.pswp__button--captions');
const currentCaption = (page: Page) =>
	page.locator('.pswp__item[aria-hidden="false"] .pswp-caption');

/** Opens an album page and returns the index of each tile that has a caption. */
async function openAlbum(page: Page, slug: string): Promise<number[]> {
	await page.goto(`/albums/${slug}`);
	await unlockAlbumIfNeeded(page, slug, pw);
	await waitForHydration(page);
	const tiles = await page
		.locator('.photo')
		.evaluateAll((els) => els.map((el) => el.querySelector('.photo-caption') !== null));
	return tiles.flatMap((captioned, i) => (captioned ? [i] : []));
}

async function openLightboxAt(page: Page, index: number) {
	await page.locator('.photo').nth(index).click();
	await expect(page.locator('.pswp--open')).toBeVisible();
}

// Album slugs paired with their captioned tiles, gathered once from the home page.
const albums: { slug: string; captioned: number[] }[] = [];
test.beforeAll(async ({ browser }) => {
	const page = await browser.newPage();
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	const cards = page.locator('.album-card');
	const slugs: string[] = [];
	for (let i = 0; i < (await cards.count()); i++) slugs.push(await slugFromCard(cards.nth(i)));
	for (const slug of slugs) albums.push({ slug, captioned: await openAlbum(page, slug) });
	await page.close();
});

const captionedAlbum = () => albums.find((a) => a.captioned.length > 0);

/** Opens the lightbox on a captioned photo, skipping when the site has none. */
async function openCaptionedPhoto(page: Page) {
	const album = captionedAlbum();
	test.skip(!album, 'no album with captions');
	await openAlbum(page, album!.slug);
	await openLightboxAt(page, album!.captioned[0]);
	await expect(currentCaption(page)).toHaveCSS('opacity', '1');
}

test('the toggle sits just right of zoom', async ({ page }) => {
	await openCaptionedPhoto(page);
	const zoom = (await page.locator('.pswp__button--zoom').boundingBox())!;
	const button = (await toggle(page).boundingBox())!;
	expect(button.x).toBeCloseTo(zoom.x + zoom.width, 0);
	const play = page.locator('.pswp__button--slideshow');
	if (await play.count()) {
		expect((await play.boundingBox())!.x).toBeCloseTo(button.x + button.width, 0);
	}
});

test('the toggle hides and shows captions', async ({ page }) => {
	await openCaptionedPhoto(page);
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Hide captions');

	await toggle(page).click();
	await expect(currentCaption(page)).toHaveCSS('opacity', '0');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Show captions');

	await toggle(page).click();
	await expect(currentCaption(page)).toHaveCSS('opacity', '1');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Hide captions');
});

test('the c key toggles captions, but not with a modifier held', async ({ page }) => {
	await openCaptionedPhoto(page);
	await page.keyboard.press('c');
	await expect(currentCaption(page)).toHaveCSS('opacity', '0');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Show captions');

	// Cmd/Ctrl+C is the browser's copy; it must not toggle.
	await page.keyboard.press('ControlOrMeta+c');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Show captions');

	await page.keyboard.press('c');
	await expect(currentCaption(page)).toHaveCSS('opacity', '1');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Hide captions');
});

test('the c key does nothing in an album without captions', async ({ page }) => {
	const album = albums.find((a) => a.captioned.length === 0);
	test.skip(!album, 'every album has captions');
	await openAlbum(page, album!.slug);
	await openLightboxAt(page, 0);
	await page.keyboard.press('c');
	expect(await page.evaluate(() => sessionStorage.getItem('ddp_captions_hidden'))).toBeNull();
});

test('hidden captions stay hidden across photos and reopening', async ({ page }) => {
	await openCaptionedPhoto(page);
	const album = captionedAlbum()!;
	await toggle(page).click();

	// Another captioned photo in the same lightbox.
	if (album.captioned.length > 1) {
		const steps = album.captioned[1] - album.captioned[0];
		for (let i = 0; i < steps; i++) await page.keyboard.press('ArrowRight');
		await expect(currentCaption(page)).toHaveCSS('opacity', '0');
	}

	// Closed and reopened: a new PhotoSwipe instance picks the choice back up.
	await page.keyboard.press('Escape');
	await expect(page.locator('.pswp')).toHaveCount(0);
	await openLightboxAt(page, album.captioned[0]);
	await expect(currentCaption(page)).toHaveCSS('opacity', '0');
	await expect(toggle(page)).toHaveAttribute('aria-label', 'Show captions');
});

test('the choice lasts for the tab session only', async ({ page, context }) => {
	await openCaptionedPhoto(page);
	await toggle(page).click();
	await expect(currentCaption(page)).toHaveCSS('opacity', '0');

	// A reload is the same session: still hidden. The URL is the photo's permalink, so the
	// lightbox reopens on its own (after decrypting, on a password-protected album).
	await page.reload();
	await expect(page.locator('.pswp--open')).toBeVisible();
	await expect(currentCaption(page)).toHaveCSS('opacity', '0');

	// Nothing is written to localStorage, so a new tab starts with captions shown.
	expect(await page.evaluate(() => localStorage.getItem('ddp_captions_hidden'))).toBeNull();
	const fresh = await context.newPage();
	await openCaptionedPhoto(fresh);
	await expect(toggle(fresh)).toHaveAttribute('aria-label', 'Hide captions');
});

test('an album without captions has no toggle', async ({ page }) => {
	const album = albums.find((a) => a.captioned.length === 0);
	test.skip(!album, 'every album has captions');
	await openAlbum(page, album!.slug);
	await openLightboxAt(page, 0);
	await expect(page.locator('.pswp__button--zoom')).toBeVisible();
	await expect(toggle(page)).toHaveCount(0);
});

test.describe('with an HTML caption', () => {
	let htmlCaption: FoundHtmlCaption | null = null;
	test.beforeAll(async ({ request }) => {
		htmlCaption = await findHtmlCaption(request);
	});

	test('links in a hidden caption cannot be clicked', async ({ page }) => {
		test.skip(!htmlCaption?.description.includes('<a'), 'no caption with a link');
		const c = htmlCaption!;
		await openAlbum(page, c.slug);
		await openLightboxAt(page, c.index);
		const link = currentCaption(page).locator('a').first();
		await expect(link).toHaveCSS('pointer-events', 'auto');

		await toggle(page).click();
		await expect(link).toHaveCSS('pointer-events', 'none');
	});
});
