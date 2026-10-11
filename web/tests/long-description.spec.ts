import { test, expect, type Page } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockAlbumIfNeeded,
	unlockSiteIfNeeded,
	albumExists
} from './helpers';

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
	await page.locator('.long-description-toggle').click();
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

// Wait out the expand/collapse animation; a click mid-animation would start the next one
// from wherever this one got to, so tests wait for each to settle.
async function settled(page: Page) {
	await expect
		.poll(() =>
			page.locator('.long-description').evaluate((el) => el.classList.contains('animating'))
		)
		.toBe(false);
}

test('long description is collapsed by default and toggles open and closed', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	await openAlbum(page, ALBUM);

	const long = page.locator('.long-description');
	const toggle = page.locator('.long-description-toggle');
	const height = async () => (await long.boundingBox())!.height;
	const lineHeight = await long.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight));
	const firstP = long.locator('p').first();
	const display = () => firstP.evaluate((el) => getComputedStyle(el).display);
	const fade = () => long.evaluate((el) => getComputedStyle(el, '::after').opacity);

	// Collapsed: two lines, paragraphs run together, fading out, with "Show more"
	await expect(long).toHaveClass(/\bclamped\b/);
	await expect(toggle).toHaveText('Show more');
	await expect(toggle).toHaveAttribute('aria-expanded', 'false');
	await expect(toggle).toHaveAttribute('aria-controls', 'long-description');
	const collapsed = await height();
	expect(collapsed).toBeLessThanOrEqual(lineHeight * 2 + 1);
	expect(await display()).toBe('inline');
	expect(await fade()).toBe('1');

	// Expanded: full text in separate paragraphs, no fade, "Close" set apart below it
	await toggle.click();
	await expect(long).not.toHaveClass(/\bclamped\b/);
	await expect(toggle).toHaveText('Close');
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	await settled(page);
	expect(await height()).toBeGreaterThan(collapsed * 3);
	expect(await display()).toBe('block');
	expect(await fade()).toBe('0');
	const gap = (await toggle.boundingBox())!.y - ((await long.boundingBox())!.y + (await height()));
	expect(gap).toBeGreaterThanOrEqual(16);

	// Collapsed again, and scrolled back into view
	await toggle.click();
	await expect(long).toHaveClass(/\bclamped\b/);
	await expect(toggle).toHaveText('Show more');
	await settled(page);
	expect(await height()).toBeCloseTo(collapsed, 0);
	await expect(long).toBeInViewport();
});

test('long description toggles instantly with reduced motion', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	await page.emulateMedia({ reducedMotion: 'reduce' });
	await openAlbum(page, ALBUM);

	const long = page.locator('.long-description');
	await page.locator('.long-description-toggle').click();
	await expect(long).not.toHaveClass(/\bclamped\b/);
	await expect(long).not.toHaveClass(/\banimating\b/);
	expect(await long.evaluate((el) => el.style.maxHeight)).toBe('');
});

test('long description short enough to fit shows no toggle', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	await openAlbum(page, ALBUM);
	await expect(page.locator('.long-description-toggle')).toBeVisible();

	// Swap in a one-line description; the overflow check should drop the toggle. Swapped
	// inside the poll because a late re-render can put the original content back.
	const long = page.locator('.long-description');
	await expect
		.poll(async () => {
			await long.evaluate((el) => {
				if (el.textContent !== 'Short.') el.innerHTML = '<p>Short.</p>';
			});
			return page.locator('.long-description-toggle').count();
		})
		.toBe(0);
	expect(await long.evaluate((el) => getComputedStyle(el, '::after').opacity)).toBe('0');
});

test('a rule sits just under the header line, above the long description', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	await openAlbum(page, ALBUM);

	const meta = (await page.locator('header .meta').boundingBox())!;
	const rule = (await page.locator('.long-description-rule').boundingBox())!;
	const long = (await page.locator('.long-description').boundingBox())!;
	const ruleGap = rule.y - (meta.y + meta.height);
	expect(ruleGap).toBeGreaterThanOrEqual(0);
	expect(ruleGap).toBeLessThan(10);
	expect(long.y).toBeGreaterThan(rule.y);
});

test('the grid does not move once an album with a long description appears', async ({ page }) => {
	test.skip(!hasAlbum, `${ALBUM} album not present`);
	// Arrive by client-side navigation, where a late toggle or an animated margin used to
	// shift the grid after it was first drawn.
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await waitForHydration(page);
	await page.locator(`a[href$="/albums/${ALBUM}"]`).first().click();
	await unlockAlbumIfNeeded(page, ALBUM, pw);
	await expect(page.locator('.gallery')).toBeVisible();

	const ys: number[] = [];
	for (let i = 0; i < 12; i++) {
		ys.push(Math.round((await page.locator('.gallery').boundingBox())!.y));
		await page.waitForTimeout(50);
	}
	expect(new Set(ys).size, `gallery y over time: ${ys.join(' ')}`).toBe(1);
});

test('album without a long description renders none', async ({ page }) => {
	test.skip(!hasPlainAlbum, `${PLAIN_ALBUM} album not present`);
	await openAlbum(page, PLAIN_ALBUM);
	await expect(page.locator('.long-description')).toHaveCount(0);
	await expect(page.locator('.long-description-rule')).toHaveCount(0);
});
