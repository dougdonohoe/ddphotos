import { test, expect } from '@playwright/test';
import {
	loadPasswords,
	unlockSite,
	unlockAlbum,
	unlockSiteIfNeeded
} from './helpers';

// Password tests — exercise the client-side encryption/decryption flow.
// All tests are skipped when PLAYWRIGHT_PASSWORDS_FILE is not set (no-password variant).
// Tests that require a site-wide password (_all_) are skipped when only per-album
// passwords are configured, and vice versa.

const pw = loadPasswords();

// First per-album-only slug (not covered by _all_): used for per-album tests.
// In pw-all, the site password also covers all albums so per-album tests
// use any album that has an explicit entry in the passwords file (e.g. "uganda").
const firstAlbumSlug = Object.keys(pw.albums)[0] ?? null;
const firstAlbumPw = firstAlbumSlug ? pw.albums[firstAlbumSlug] : null;

// --- Site-wide password tests ---

test('site password prompt is shown on home page', async ({ page }) => {
	test.skip(!pw.all, 'no site-wide password configured');
	await page.goto('/');
	await expect(page.locator('input[type="password"]')).toBeVisible();
	// Albums must NOT be visible before unlocking
	await expect(page.locator('.albums')).not.toBeVisible();
});

test('wrong site password keeps prompt visible', async ({ page }) => {
	test.skip(!pw.all, 'no site-wide password configured');
	await page.goto('/');
	await page.locator('input[type="password"]').fill('wrongpassword');
	await page.locator('button[type="submit"]').click();
	// Prompt stays visible; albums stay hidden
	await expect(page.locator('input[type="password"]')).toBeVisible();
	await expect(page.locator('.albums')).not.toBeVisible();
});

test('correct site password unlocks home page and shows title', async ({ page }) => {
	test.skip(!pw.all, 'no site-wide password configured');
	await page.goto('/');
	await unlockSite(page, pw.all!);
	await expect(page.locator('.albums')).toBeVisible();
	await expect(page.locator('h1')).toBeVisible();
});

test('site password is remembered on reload', async ({ page }) => {
	test.skip(!pw.all, 'no site-wide password configured');
	await page.goto('/');
	await unlockSite(page, pw.all!);
	await page.reload();
	// Albums should appear without re-entering the password
	await expect(page.locator('.albums')).toBeVisible();
	await expect(page.locator('input[type="password"]')).not.toBeVisible();
});

// --- Per-album password tests ---

test('album password prompt is shown when navigating to locked album', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	// Unlock site first if required so we can navigate to the album
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	await expect(page.locator('input[type="password"]')).toBeVisible();
	await expect(page.locator('.gallery')).not.toBeVisible();
});

test('wrong album password keeps prompt visible', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	await page.locator('input[type="password"]').fill('wrongpassword');
	await page.locator('button[type="submit"]').click();
	await expect(page.locator('input[type="password"]')).toBeVisible();
	await expect(page.locator('.gallery')).not.toBeVisible();
});

test('correct album password unlocks album', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	await unlockAlbum(page, firstAlbumPw!);
	await expect(page.locator('.gallery')).toBeVisible();
});

test('album password is remembered on reload', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	await unlockAlbum(page, firstAlbumPw!);
	await page.reload();
	// Gallery should appear without re-entering the password
	await expect(page.locator('.gallery')).toBeVisible();
	await expect(page.locator('input[type="password"]')).not.toBeVisible();
});

test('album cover appears on home page after unlocking album', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	// Unlock site if needed, then unlock the album
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	await unlockAlbum(page, firstAlbumPw!);
	// Return to home page (password is now cached in localStorage)
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	// The album card's cover placeholder should now show a background image
	const card = page.locator(`.album-card[href="/albums/${firstAlbumSlug}"]`);
	const placeholder = card.locator('.album-cover-placeholder');
	await expect(placeholder).toHaveCSS('background-image', /url\(/);
});

// --- ?clear tests ---

test('?clear removes site password and shows prompt again', async ({ page }) => {
	test.skip(!pw.all, 'no site-wide password configured');
	// Unlock, then clear
	await page.goto('/');
	await unlockSite(page, pw.all!);
	await expect(page.locator('.albums')).toBeVisible();
	await page.goto('/?clear');
	// ?clear redirects to / and password should be required again
	await expect(page).toHaveURL('/');
	await expect(page.locator('input[type="password"]')).toBeVisible();
});

test('?clear removes album password and shows prompt again', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	// Skip if site is also encrypted — ?clear test is simpler to isolate per-album only
	test.skip(!!pw.all, 'use site-password ?clear test for pw-all variant');
	// Unlock the album, confirm it works, then clear
	await page.goto(`/albums/${firstAlbumSlug}`);
	await unlockAlbum(page, firstAlbumPw!);
	await expect(page.locator('.gallery')).toBeVisible();
	await page.goto('/?clear');
	await expect(page).toHaveURL('/');
	// Album password should be gone — visiting the album should prompt again
	await page.goto(`/albums/${firstAlbumSlug}`);
	await expect(page.locator('input[type="password"]')).toBeVisible();
});

// --- Password prompt dialog behavior ---

test('password dialog has autofocus on input', async ({ page }) => {
	test.skip(!pw.all && !firstAlbumSlug, 'no passwords configured');
	if (pw.all) {
		await page.goto('/');
	} else {
		await page.goto(`/albums/${firstAlbumSlug}`);
	}
	await expect(page.locator('input[type="password"]')).toBeFocused();
});

test('password dialog title uses album title not slug', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	await page.goto(`/albums/${firstAlbumSlug}`);
	// Dialog title should be a properly-capitalized name, not a raw slug
	// e.g. "Uganda requires a password." not "uganda requires a password."
	const heading = page.locator('.card h2');
	await expect(heading).toBeVisible();
	const text = await heading.innerText();
	// First character of the album name must be uppercase
	expect(text[0]).toMatch(/[A-Z]/);
	// Must not contain hyphens (slug form) — real titles use spaces or proper capitalization
	expect(text).not.toMatch(/-/);
});

// --- Album cover NOT shown before unlock (per-album encrypted) ---

test('per-album encrypted album shows lock icon not cover before unlock', async ({ page }) => {
	test.skip(!firstAlbumSlug, 'no per-album password configured');
	// Only meaningful when the home page is NOT site-encrypted (so we can see cards)
	test.skip(!!pw.all, 'home page requires site password; skip for pw-all variant');
	await page.goto('/');
	const card = page.locator(`.album-card[href="/albums/${firstAlbumSlug}"]`);
	// Lock icon should be visible (cover is not included in index for per-album encrypted albums)
	await expect(card.locator('.ddp-lock-icon')).toBeVisible();
	// Placeholder should NOT have a background image pointing to a photo
	const placeholder = card.locator('.album-cover-placeholder');
	const bgImage = await placeholder.evaluate((el) => getComputedStyle(el).backgroundImage);
	// Either no background-image at all, or 'none'
	expect(['none', '']).toContain(bgImage.trim() === '' ? '' : bgImage === 'none' ? 'none' : 'other');
});
