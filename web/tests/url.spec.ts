import { test, expect } from '@playwright/test';
import { waitForHydration } from './helpers';

// URL management tests — covers the replaceState fixes:
//  - history.replaceState was replaced with SvelteKit's replaceState from $app/navigation
//    to prevent SvelteKit treating each photo navigation as a full load() re-run.
//  - The initial replaceState is guarded with `if (animate)` so permalink opens
//    (animate=false) don't call replaceState before the router is initialised.

test('opening a photo updates URL to permalink', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(/\/albums\/antarctica\/\d+/);
});

test('navigating photos updates URL', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica/1');

	await page.locator('.pswp__button--arrow--next').click();
	await expect(page).toHaveURL('/albums/antarctica/2');
});

test('closing lightbox restores album URL', async ({ page }) => {
	await page.goto('/albums/antarctica');
	await waitForHydration(page);
	await page.locator('.photo').nth(0).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page).toHaveURL(/\/albums\/antarctica\/\d+/);

	await page.keyboard.press('Escape');
	await expect(page.locator('.pswp')).not.toBeVisible();
	await expect(page).toHaveURL('/albums/antarctica');
});

test('loading a permalink URL opens the correct photo', async ({ page }) => {
	await page.goto('/albums/antarctica/14');
	await expect(page.locator('.pswp')).toBeVisible();
	// URL should stay at the permalink (not be rewritten by router init)
	await expect(page).toHaveURL('/albums/antarctica/14');
});
