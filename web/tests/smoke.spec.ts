import { test, expect } from '@playwright/test';
import { loadPasswords, unlockSiteIfNeeded, unlockAlbumIfNeeded, albumExists } from './helpers';

// Smoke tests — basic rendering checks that catch build/deploy regressions.
// Albums checked here (Antarctica, Uganda) are present in both sample and prod.

const pw = loadPasswords();

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
});

test(
	'home page lists albums including known overlap albums',
	{ tag: '@deploy' },
	async ({ page }) => {
		test.skip(!hasAntarctica, 'antarctica album not present');
		await page.goto('/');
		await unlockSiteIfNeeded(page, pw);
		// Spot-check albums present in both sample and prod
		await expect(page.locator('.album-card', { hasText: 'Antarctica' })).toBeVisible();
		await expect(page.locator('.album-card', { hasText: 'Uganda' })).toBeVisible();
	}
);

test('home page album cards show description', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	// Antarctica's description is stable across sample and prod
	await expect(page.locator('.album-card', { hasText: 'bottom of the world' })).toBeVisible();
});

test('album page shows title, description, and photo count', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await expect(page.locator('h1')).toHaveText('Antarctica');
	await expect(page.locator('.description')).toContainText('bottom of the world');
	await expect(page.locator('.meta')).toContainText(/\d+ photos/);
});

test('album page renders photos in the grid', { tag: '@deploy' }, async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	// At least one photo button should be present
	await expect(page.locator('.photo').first()).toBeVisible();
});

// The test above proves the grid renders, but it renders from index.json alone: a tile is
// visible whether its WebP exists. That distinction does not matter much in dev,
// where the images sit next to the JSON, but it is exactly what a deploy gets wrong, since
// bin/deploy-photos.sh syncs album media and album metadata in separate passes. Decoding
// the image is the only assertion in the suite that fails when the bytes are missing, which
// is why this carries @deploy.
test('album page grid images actually load', { tag: '@deploy' }, async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);

	const img = page.locator('.photo img').first();
	await expect(img).toBeVisible();
	// naturalWidth stays 0 for an <img> whose src 404s or decodes as something other than
	// an image, so this catches a missing or corrupt WebP that toBeVisible() does not.
	await expect
		.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth), { timeout: 10_000 })
		.toBeGreaterThan(0);
});

test('album page has correct Open Graph tags', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', /^Antarctica/);
	await expect(page.locator('meta[property="og:type"]')).toHaveAttribute('content', 'website');
	// The album's own description wins over the site description, which is only the fallback
	// for an album that has none.
	await expect(page.locator('meta[property="og:description"]')).toHaveAttribute(
		'content',
		/bottom of the world/
	);
	// og:image must be a JPEG (not WebP) — iMessage and many crawlers don't support WebP previews
	await expect(page.locator('meta[property="og:image"]')).toHaveAttribute(
		'content',
		/antarctica\/cover\.jpg$/
	);
});

test('home page og:site_name matches siteName from config.json', async ({ page }) => {
	const config = await page.request.get('/albums/config.json').then((r) => r.json());
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await expect(page.locator('meta[property="og:site_name"]')).toHaveAttribute(
		'content',
		config.siteName
	);
});

test('album page og:site_name matches siteName from config.json', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	const config = await page.request.get('/albums/config.json').then((r) => r.json());
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await expect(page.locator('meta[property="og:site_name"]')).toHaveAttribute(
		'content',
		config.siteName
	);
});

test('?boom on home page shows 500 error card', async ({ page }) => {
	await page.goto('/?boom');
	await expect(page.locator('.card-title')).toContainText('500 Error');
});

test(
	'home page has Open Graph image tag pointing to a JPEG',
	{ tag: '@deploy' },
	async ({ page }) => {
		// In the pw-all variant every album is encrypted, so the home page derives no OG
		// cover image (it only uses non-encrypted albums). Skip rather than expect a tag
		// that won't be present by design.
		test.skip(!!pw.all, 'pw-all: all albums encrypted, no home OG image available');

		await page.goto('/');
		await unlockSiteIfNeeded(page, pw);
		// Must be a JPEG for iMessage / crawler compatibility — not WebP.
		// May be hero.jpg (when a hero is configured) or an album cover.jpg.
		await expect(page.locator('meta[property="og:image"]')).toHaveAttribute('content', /\.jpg$/);
	}
);

test('lightbox top bar has a shade behind its controls', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await page.locator('.gallery.layout-ready').waitFor();
	await page.locator('.photo').first().click();
	await expect(page.locator('.pswp')).toBeVisible();

	// The counter and the zoom/copy-link/close buttons are white with no background of
	// their own, so against a bright photo (a pale sky, a whitewashed wall) they were
	// invisible. A gradient on .pswp__top-bar::before fixes that; assert it is really
	// applied, since a purely cosmetic rule is easy to drop by accident.
	const bg = await page
		.locator('.pswp__top-bar')
		.evaluate((el) => getComputedStyle(el, '::before').backgroundImage);
	expect(bg).toContain('gradient');
	expect(bg).not.toBe('none');
});
