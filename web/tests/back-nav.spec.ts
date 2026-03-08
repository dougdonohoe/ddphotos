import { test, expect } from '@playwright/test';
import { waitForHydration } from './helpers';

// Browser back/forward navigation tests for the lightbox.
//
// The core requirement: pressing browser back while the lightbox is open should
// close the lightbox and return to the album URL, NOT navigate to a prior page
// (e.g. the album list) or leave the lightbox hanging open.

test('back button after opening photo closes lightbox and returns to album URL', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);

	await page.locator('.photo').nth(1).click(); // open photo 2
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/2');

	await page.goBack();

	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');
});

test('back button after navigating photos in lightbox closes lightbox', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);

	await page.locator('.photo').nth(0).click(); // open photo 1
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/1');

	await page.locator('.pswp__button--arrow--next').click(); // advance to photo 2
	await expect(page).toHaveURL('/albums/antarctica/2');

	await page.goBack();

	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');
});

test('back button after closing lightbox navigates to previous page', async ({ page }) => {
	await page.goto('/');
	await page.locator('.album-card', { hasText: 'Antarctica' }).click();
	await waitForHydration(page);

	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();

	// Close lightbox normally (Escape key)
	await page.keyboard.press('Escape');
	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');

	// Back should now go to the album list, not reopen lightbox
	await page.goBack();
	await expect(page).toHaveURL('/');
	await expect(page.locator('.pswp')).not.toBeVisible();
});

test('after reload and back, album photos render correctly', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);

	// Wait for images to start loading (confirms imageSrcs are populated)
	await expect(page.locator('.photo img').first()).toHaveAttribute('src', /\.webp/);

	// Open lightbox
	await page.locator('.photo').nth(1).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/2');

	// Reload at the permalink URL — lightbox should reopen
	await page.reload();
	await expect(page.locator('.pswp')).toBeVisible();

	// Press back
	await page.goBack();

	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');

	// Photos must actually render — not stuck as invisible placeholders.
	// Bug: $effect reset imageLoaded to false on same-album re-fetch; src attrs
	// didn't change so onload never re-fired, leaving images at opacity 0.
	await expect(page.locator('.photo img.loaded').first()).toBeVisible({ timeout: 8000 });
});

test('URL never shows /albums/undefined', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);

	await page.locator('.photo').nth(1).click();
	await expect(page.locator('.pswp')).toBeVisible();

	await page.goBack();

	// The slug must never become "undefined"
	await expect(page).not.toHaveURL(/\/albums\/undefined/);
});
