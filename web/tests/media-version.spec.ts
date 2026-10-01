import { test, expect } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockAlbumIfNeeded,
	unlockSiteIfNeeded,
	albumExists
} from './helpers';

const pw = loadPasswords();

// WebPs and MP4s are served as immutable but keep their names when photogen regenerates
// them, so the site requests each with the ?v= version from index.json (albumFileUrl).
// Without it, a browser shows a regenerated photo's old copy for up to a year.
const VERSIONED = /\.webp\?v=[0-9a-f]{8}$/;

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
});

test('grid and lightbox images carry the version from index.json', async ({ page, request }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');

	// Exact values need the plain index; an encrypted album's is only readable in the page,
	// so there the format alone is checked.
	const resp = await request.get('/albums/antarctica/index.json');
	const index = resp.ok() ? await resp.json().catch(() => null) : null;
	const first = index?.photos?.[0];

	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await waitForHydration(page);

	const grid = page.locator('.photo img').first();
	await expect(grid).toHaveAttribute('src', VERSIONED);
	if (first) {
		await expect(grid).toHaveAttribute(
			'src',
			`/albums/antarctica/${first.src.grid}?v=${first.version}`
		);
	}

	// Every image PhotoSwipe loads, the grid placeholder and neighbouring slides included.
	await page.locator('.photo').first().click();
	if (first) {
		await expect(
			page.locator(`.pswp__img[src="/albums/antarctica/${first.src.full}?v=${first.version}"]`)
		).toBeAttached();
	}
	// PhotoSwipe creates an img before it assigns the src, hence [src].
	await expect(page.locator('.pswp__img[src]').first()).toBeAttached();
	for (const src of await page
		.locator('.pswp__img[src]')
		.evaluateAll((els) => els.map((el) => el.getAttribute('src')))) {
		expect(src).toMatch(VERSIONED);
	}
});

test('home page album covers carry their version', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await waitForHydration(page);

	// Per-album encrypted albums show no <img> cover before unlock; any cover shown is versioned.
	const covers = page.locator('.album-card img');
	test.skip((await covers.count()) === 0, 'no album shows a cover image');
	await expect(covers.first()).toHaveAttribute('src', VERSIONED);
});
