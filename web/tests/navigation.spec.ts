import { test, expect } from '@playwright/test';
import { waitForHydration } from './helpers';

// Cross-album navigation tests — covers the $effect fix for stale imageSrcs.
//
// SvelteKit reuses the same component instance when navigating between albums
// (same [slug]/[[index]] route). Before the fix, `imageSrcs` was only populated
// in onMount (which doesn't re-run on client-side nav), so navigating from album
// A to album B would show A's photos under B's title and description.
//
// Album names and slugs are read dynamically from the home page so these tests
// work against any site (sample, dev, or prod) without hardcoding album names.

test('navigating from one album to another shows correct content', async ({ page }) => {
	await page.goto('/');

	// Read first two album names dynamically
	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const secondName = await cards.nth(1).locator('h2').innerText();

	// Load first album via full page load
	await cards.nth(0).click();
	await expect(page.locator('h1')).toHaveText(firstName);

	// Client-side navigate to second album via the back link + album card click
	await page.locator('header a', { hasText: '← Albums' }).click();
	await page.locator('.album-card', { hasText: secondName }).click();

	// Title must reflect the new album, not the previous one
	await expect(page.locator('h1')).toHaveText(secondName);
});

test('lightbox works correctly after cross-album navigation', async ({ page }) => {
	await page.goto('/');

	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const secondName = await cards.nth(1).locator('h2').innerText();
	const secondSlug = (await cards.nth(1).getAttribute('href'))!.replace('/albums/', '');

	await cards.nth(0).click();
	await expect(page.locator('h1')).toHaveText(firstName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await page.locator('.album-card', { hasText: secondName }).click();
	await expect(page.locator('h1')).toHaveText(secondName);

	// Open a photo — if imageSrcs wasn't reset, wrong images would be loaded
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	// URL must reflect the new album (not the old one)
	await expect(page).toHaveURL(new RegExp(`/albums/${secondSlug}/\\d+`));
});

test('navigating through multiple albums maintains correct state', async ({ page }) => {
	await page.goto('/');

	const cards = page.locator('.album-card');
	const firstName = await cards.nth(0).locator('h2').innerText();
	const secondName = await cards.nth(1).locator('h2').innerText();
	const thirdName = await cards.nth(2).locator('h2').innerText();
	const thirdSlug = (await cards.nth(2).getAttribute('href'))!.replace('/albums/', '');

	await cards.nth(0).click();
	await expect(page.locator('h1')).toHaveText(firstName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await page.locator('.album-card', { hasText: secondName }).click();
	await expect(page.locator('h1')).toHaveText(secondName);

	await page.locator('header a', { hasText: '← Albums' }).click();
	await page.locator('.album-card', { hasText: thirdName }).click();
	await expect(page.locator('h1')).toHaveText(thirdName);

	// Open lightbox — should be third album's photos, not first or second
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(new RegExp(`/albums/${thirdSlug}/\\d+`));
});
