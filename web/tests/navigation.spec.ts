import { test, expect } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockSiteIfNeeded,
	unlockAlbumIfNeeded,
	slugFromCard,
	hasAtLeastNAlbums
} from './helpers';

// Cross-album navigation tests — covers the $effect fix for stale imageSrcs.
//
// SvelteKit reuses the same component instance when navigating between albums
// (same [slug]/[[index]] route). Before the fix, `imageSrcs` was only populated
// in onMount (which doesn't re-run on client-side nav), so navigating from album
// A to album B would show A's photos under B's title and description.
//
// Album names and slugs are read dynamically from the home page so these tests
// work against any site (sample, dev, or prod) without hardcoding album names.

const pw = loadPasswords();

let threeAlbums = true;
test.beforeAll(async ({ request }) => {
	threeAlbums = await hasAtLeastNAlbums(request, 3);
});

test('navigating from one album to another shows correct content', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);

	// Read first two album names and slugs dynamically
	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const firstSlug = await slugFromCard(cards.nth(0));
	const secondName = await cards.nth(1).locator('h2').innerText();
	const secondSlug = await slugFromCard(cards.nth(1));

	// Load first album via full page load
	await cards.nth(0).click();
	await unlockAlbumIfNeeded(page, firstSlug, pw);
	await expect(page.locator('h1')).toHaveText(firstName);

	// Client-side navigate to second album via the back link + album card click
	await page.locator('header a', { hasText: '← Albums' }).click();
	await unlockSiteIfNeeded(page, pw);
	await page.locator('.album-card', { hasText: secondName }).click();
	await unlockAlbumIfNeeded(page, secondSlug, pw);

	// Title must reflect the new album, not the previous one
	await expect(page.locator('h1')).toHaveText(secondName);
});

test('lightbox works correctly after cross-album navigation', async ({ page }) => {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);

	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const firstSlug = await slugFromCard(cards.nth(0));
	const secondName = await cards.nth(1).locator('h2').innerText();
	const secondSlug = await slugFromCard(cards.nth(1));

	await cards.nth(0).click();
	await unlockAlbumIfNeeded(page, firstSlug, pw);
	await expect(page.locator('h1')).toHaveText(firstName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await unlockSiteIfNeeded(page, pw);
	await page.locator('.album-card', { hasText: secondName }).click();
	await unlockAlbumIfNeeded(page, secondSlug, pw);
	await expect(page.locator('h1')).toHaveText(secondName);

	// Open a photo — if imageSrcs wasn't reset, wrong images would be loaded
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	// URL must reflect the new album (not the old one)
	await expect(page).toHaveURL(new RegExp(`/albums/${secondSlug}/\\d+`));
});

test('navigating through multiple albums maintains correct state', async ({ page }) => {
	test.skip(!threeAlbums, 'fewer than 3 albums');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);

	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const firstSlug = await slugFromCard(cards.nth(0));
	const secondName = await cards.nth(1).locator('h2').innerText();
	const secondSlug = await slugFromCard(cards.nth(1));
	const thirdName = await cards.nth(2).locator('h2').innerText();
	const thirdSlug = await slugFromCard(cards.nth(2));

	await cards.nth(0).click();
	await unlockAlbumIfNeeded(page, firstSlug, pw);
	await expect(page.locator('h1')).toHaveText(firstName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await unlockSiteIfNeeded(page, pw);
	await page.locator('.album-card', { hasText: secondName }).click();
	await unlockAlbumIfNeeded(page, secondSlug, pw);
	await expect(page.locator('h1')).toHaveText(secondName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await unlockSiteIfNeeded(page, pw);
	await page.locator('.album-card', { hasText: thirdName }).click();
	await unlockAlbumIfNeeded(page, thirdSlug, pw);
	await expect(page.locator('h1')).toHaveText(thirdName);

	// Open lightbox — should be third album's photos, not first or second
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(new RegExp(`/albums/${thirdSlug}/\\d+`));
});
