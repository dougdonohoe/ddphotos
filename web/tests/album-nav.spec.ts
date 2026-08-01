import { test, expect } from '@playwright/test';
import {
	loadPasswords,
	siteAlbumNav,
	slugFromCard,
	unlockSiteIfNeeded,
	unlockAlbumIfNeeded
} from './helpers';

// customizations.album_nav tests — the configured links replace the default "← Albums"
// link in each album page header.
//
// Presence is derived from config.json, so these tests run against any site: the
// sample/config-album-nav variant (bin/run-tests.sh --album-nav) exercises the
// configured path, every other variant exercises the default.

const pw = loadPasswords();

/** Open the first album and return once its header is rendered. */
async function gotoFirstAlbum(page: import('@playwright/test').Page) {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	const card = page.locator('.album-card').first();
	const slug = await slugFromCard(card);
	await card.click();
	await unlockAlbumIfNeeded(page, slug, pw);
	await expect(page.locator('header h1')).toBeVisible();
}

test('configured album_nav links render in the album header', async ({ page, request }) => {
	const nav = await siteAlbumNav(request);
	test.skip(nav.length === 0, 'no album_nav configured');

	await gotoFirstAlbum(page);

	const links = page.locator('header nav.album-nav a');
	await expect(links).toHaveCount(nav.length);
	for (const [i, link] of nav.entries()) {
		await expect(links.nth(i)).toHaveText(link.label);
	}
});

test('album_nav replaces the default back link', async ({ page, request }) => {
	const nav = await siteAlbumNav(request);
	test.skip(nav.length === 0, 'no album_nav configured');

	await gotoFirstAlbum(page);

	await expect(page.locator('header a', { hasText: '← Albums' })).toHaveCount(0);
});

test('album_nav ids and new_tab reach the rendered anchors', async ({ page, request }) => {
	const nav = await siteAlbumNav(request);
	test.skip(nav.length === 0, 'no album_nav configured');

	await gotoFirstAlbum(page);

	for (const link of nav) {
		// Every link in the fixture has an id; skip any that don't so this stays generic.
		if (!link.id) continue;
		const anchor = page.locator(`header nav.album-nav a#${link.id}`);
		await expect(anchor).toHaveCount(1);
		// Without new_tab the attributes are absent entirely, not empty.
		if (link.newTab) {
			await expect(anchor).toHaveAttribute('target', '_blank');
			await expect(anchor).toHaveAttribute('rel', 'noopener');
		} else {
			await expect(anchor).not.toHaveAttribute('target');
			await expect(anchor).not.toHaveAttribute('rel');
		}
	}
});

test('an internal album_nav link navigates back to the home page', async ({ page, request }) => {
	const nav = await siteAlbumNav(request);
	const internal = nav.find((l) => l.href.startsWith('/'));
	test.skip(!internal, 'no internal album_nav link configured');

	await gotoFirstAlbum(page);

	await page.locator(`header nav.album-nav a[href="${internal!.href}"]`).click();
	await unlockSiteIfNeeded(page, pw);
	await expect(page.locator('.album-card').first()).toBeVisible();
	expect(new URL(page.url()).pathname).toBe('/');
});

test('default back link is used when album_nav is not configured', async ({ page, request }) => {
	const nav = await siteAlbumNav(request);
	test.skip(nav.length > 0, 'album_nav is configured — skipping default check');

	await gotoFirstAlbum(page);

	await expect(page.locator('header nav.album-nav')).toHaveCount(0);
	await expect(page.locator('header a', { hasText: '← Albums' })).toHaveCount(1);
});
