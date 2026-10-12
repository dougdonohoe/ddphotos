import { test, expect, devices } from '@playwright/test';
import { waitForHydration, loadPasswords, unlockAlbumIfNeeded, albumExists } from './helpers';

const pw = loadPasswords();

let hasAntarctica = true;
test.beforeAll(async ({ request }) => {
	hasAntarctica = await albumExists(request, 'antarctica');
});

// URL management tests — covers the shallow routing fixes:
//  - history.replaceState was replaced with SvelteKit's shallow goto from $app/navigation
//    to prevent SvelteKit treating each photo navigation as a full load() re-run.
//  - The initial history push is guarded with `if (animate)` so permalink opens
//    (animate=false) don't navigate before the router is initialised.

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

// Each slide change starts a shallow goto, and one that starts before the last has
// finished supersedes it. The URL must still land on the last photo, with a single
// history entry for the open lightbox.
test('rapid photo changes leave the URL on the last photo', async ({ page }) => {
	test.skip(!hasAntarctica, 'antarctica album not present');
	await page.goto('/albums/antarctica');
	await unlockAlbumIfNeeded(page, 'antarctica', pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/1');

	for (let i = 0; i < 5; i++) await page.keyboard.press('ArrowRight');
	await expect(page).toHaveURL('/albums/antarctica/6');

	await page.goBack();
	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');
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

// On a phone the pre-rendered gallery is laid out 1200px wide, wider than the screen, so the
// browser zooms out to fit it until onMount re-lays it out. A permalink must not open the
// lightbox before then, or PhotoSwipe sizes itself to the zoomed-out viewport and centers
// the photo below the bottom of the screen.
test.describe('on a phone', () => {
	// Not the whole descriptor: its defaultBrowserType (webkit) can't be set in a describe.
	const { viewport, userAgent, deviceScaleFactor, isMobile, hasTouch } = devices['iPhone 15 Pro'];
	test.use({ viewport, userAgent, deviceScaleFactor, isMobile, hasTouch });

	test('loading a permalink shows the photo on screen', async ({ page }) => {
		test.skip(!hasAntarctica, 'antarctica album not present');
		if (pw.all || pw.albums['antarctica']) {
			await page.goto('/albums/antarctica');
			await unlockAlbumIfNeeded(page, 'antarctica', pw);
		}
		await page.goto('/albums/antarctica/4');
		// Photo 4's caption, which is also its alt text.
		const photo = page.locator('.pswp__img[alt="Last one in is a rotten egg!"]');
		await expect(photo).toBeVisible();
		const { height } = page.viewportSize()!;
		await expect
			.poll(async () => {
				const box = await photo.boundingBox();
				return box !== null && box.y >= 0 && box.y + box.height <= height;
			})
			.toBe(true);
	});
});
