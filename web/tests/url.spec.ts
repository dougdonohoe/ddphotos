import { test, expect } from '@playwright/test';
import { waitForHydration, loadPasswords, unlockAlbumIfNeeded, albumExists } from './helpers';

const pw = loadPasswords();

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
});

// URL management tests — covers the replaceState fixes:
//  - history.replaceState was replaced with SvelteKit's replaceState from $app/navigation
//    to prevent SvelteKit treating each photo navigation as a full load() re-run.
//  - The initial replaceState is guarded with `if (animate)` so permalink opens
//    (animate=false) don't call replaceState before the router is initialised.

test('opening a photo updates URL to permalink', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(/\/albums\/antarctica\/\d+/);
});

test('navigating photos updates URL', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/1');

	await page.locator('.pswp__button--arrow--next').click();
	await expect(page).toHaveURL('/albums/antarctica/2');
});

test('closing lightbox restores album URL', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(/\/albums\/antarctica\/\d+/);

	await page.keyboard.press('Escape');
	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');
});

test('loading a permalink URL opens the correct photo', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	// If the album is encrypted, unlock at the base URL first so the password is
	// stored in localStorage. Then navigating to the permalink triggers tryDecryptAlbum
	// in onMount, which auto-decrypts and opens the lightbox. (handleUnlock, called by
	// the password form submit, does not call openLightbox — only tryDecryptAlbum does.)
	if (pw.all || pw.albums['antarctica']) {
		await page.goto('/albums/antarctica');
		await unlockAlbumIfNeeded(page, 'antarctica', pw);
	}
	await page.goto('/albums/antarctica/14');
	await expect(page.locator('.pswp')).toBeVisible();
	// URL should stay at the permalink (not be rewritten by router init)
	await expect(page).toHaveURL('/albums/antarctica/14');
});
