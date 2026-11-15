import { test, expect } from '@playwright/test';
import { loadPasswords, unlockSiteIfNeeded, waitForHydration } from './helpers';

// Album card structure. Descriptions are HTML and may hold links, so a card must not be an
// <a>: an <a> inside an <a> makes the HTML parser split the pre-rendered card (cover in one
// grid cell, title and description in the next), which shows as a flash until hydration
// re-renders the grid. Content-driven: the sample's 'the-way' description holds a link.

const pw = loadPasswords();

test.describe('pre-rendered HTML, before hydration', () => {
	test.use({ javaScriptEnabled: false });

	test('each album card holds its own title', async ({ page }) => {
		test.skip(!!pw.all, 'home page albums are encrypted, not pre-rendered');
		await page.goto('/');
		const cards = page.locator('.albums > *');
		const count = await cards.count();
		expect(count).toBeGreaterThan(0);
		for (let i = 0; i < count; i++) {
			const card = cards.nth(i);
			await expect(card).toHaveClass(/\balbum-card\b/);
			await expect(card.locator('h2')).toHaveCount(1);
		}
	});
});

test('a link in a card description opens its own target, not the album', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await waitForHydration(page);
	const link = page.locator('.album-card .description a').first();
	test.skip((await link.count()) === 0, 'no album description holds a link');
	expect(await link.getAttribute('href')).not.toMatch(/^\/albums\//);
	// A trial click runs Playwright's hit-test without clicking: it fails if the title link's
	// stretched ::after sits on top of the description link and would take the click.
	await link.click({ trial: true, timeout: 5000 });
});

test('clicking a card outside its title opens the album', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await waitForHydration(page);
	const card = page.locator('.album-card').first();
	const href = await card.locator('.album-link').getAttribute('href');
	// Click by position: the stretched title link covers .meta, so an element click on .meta
	// would fail Playwright's hit-test, which is the point of the stretched link.
	const box = (await card.locator('.meta').boundingBox())!;
	await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
	await expect(page).toHaveURL(new RegExp(`${href}$`));
});

test('arrow keys move focus between album cards', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await waitForHydration(page);
	const links = page.locator('.album-card .album-link');
	test.skip((await links.count()) < 2, 'needs two albums');
	await links.nth(0).focus();
	await page.keyboard.press('ArrowRight');
	await expect(links.nth(1)).toBeFocused();
	await page.keyboard.press('ArrowLeft');
	await expect(links.nth(0)).toBeFocused();
});
