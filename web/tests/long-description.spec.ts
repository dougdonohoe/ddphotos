import { test, expect, type Page } from '@playwright/test';
import { waitForHydration, loadPasswords, unlockAlbumIfNeeded, albumExists } from './helpers';

const pw = loadPasswords();

// The sample site's the-way album carries a long_description_html; uganda has none.
const ALBUM = 'the-way';
const PLAIN_ALBUM = 'uganda';
const DESKTOP = { width: 1280, height: 900 };
const MOBILE = { width: 390, height: 844 };

let hasAlbum = true;
let hasPlainAlbum = true;
test.beforeAll(async ({ request }) => {
	hasAlbum = await albumExists(request, ALBUM);
	hasPlainAlbum = await albumExists(request, PLAIN_ALBUM);
});

async function openAlbum(page: Page, slug: string) {
	await page.goto(`/albums/${slug}`);
	await unlockAlbumIfNeeded(page, slug, pw);
	await waitForHydration(page);
	await expect(page.locator('.gallery')).toBeVisible();
}

test('long description renders HTML between the header and the grid', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	await openAlbum(page, ALBUM);

	const long = page.locator('.long-description');
	await expect(long).toBeVisible();
	expect(await long.locator('p').count()).toBeGreaterThan(1);
	await expect(long.locator('a[href*="medium.com"]')).toBeVisible();

	// header, then the long description, then the grid
	const order = await page.evaluate(() => {
		const main = document.querySelector('main')!;
		const kids = Array.from(main.children);
		const at = (sel: string) => kids.indexOf(main.querySelector(`:scope > ${sel}`)!);
		return { header: at('header'), long: at('.long-description'), gallery: at('.gallery') };
	});
	expect(order.header).toBeGreaterThanOrEqual(0);
	expect(order.long).toBeGreaterThan(order.header);
	expect(order.gallery).toBeGreaterThan(order.long);
});

for (const [name, viewport] of [
	['desktop', DESKTOP],
	['mobile', MOBILE]
] as const) {
	test(`long description lines up with the grid edges (${name})`, async ({ page }) => {
		test.skip(!hasAlbum, `${ALBUM} album not present`);
		await page.setViewportSize(viewport);
		await openAlbum(page, ALBUM);

		const long = (await page.locator('.long-description').boundingBox())!;
		const grid = (await page.locator('.gallery').boundingBox())!;
		expect(Math.abs(long.x - grid.x)).toBeLessThanOrEqual(1);
		expect(Math.abs(long.x + long.width - (grid.x + grid.width))).toBeLessThanOrEqual(1);
	});
}

for (const theme of ['dark', 'light'] as const) {
	test(`long description uses the body text color and a mid-size font (${theme})`, async ({
		page
	}) => {
		test.skip(!hasAlbum, `${ALBUM} album not present`);
		await openAlbum(page, ALBUM);
		await page.evaluate((t) => document.documentElement.setAttribute('data-theme', t), theme);

		const styles = await page.evaluate(() => {
			const size = (sel: string) =>
				parseFloat(getComputedStyle(document.querySelector(sel)!).fontSize);
			// Resolve --text-color to the same rgb() form getComputedStyle reports
			const probe = document.createElement('span');
			probe.style.color = 'var(--text-color)';
			document.body.appendChild(probe);
			const textColor = getComputedStyle(probe).color;
			probe.remove();
			return {
				long: size('.long-description'),
				title: size('header h1'),
				description: size('header .description'),
				color: getComputedStyle(document.querySelector('.long-description')!).color,
				textColor
			};
		});
		expect(styles.long).toBeGreaterThan(styles.description);
		expect(styles.long).toBeLessThan(styles.title);
		expect(styles.color).toBe(styles.textColor);
	});
}

test('album without a long description renders none', async ({ page }) => {
	test.skip(!hasPlainAlbum, `${PLAIN_ALBUM} album not present`);
	await openAlbum(page, PLAIN_ALBUM);
	await expect(page.locator('.long-description')).toHaveCount(0);
});
