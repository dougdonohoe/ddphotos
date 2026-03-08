/**
 * Capture screenshots of the photo site using Playwright.
 *
 * Usage:
 *   node web/scripts/screenshots.mjs [options]
 *
 * Options:
 *   --base-url <url>   Base URL of the running site (default: http://localhost:8080)
 *   --album <slug>     Album slug to use for album/lightbox screenshots
 *                      (default: first album found on the home page)
 *   --out <dir>        Output directory (default: screenshots/)
 *   --photo <n>        1-based photo number to open in lightbox (default: 1)
 *
 * Requires a running server (Docker Apache or Vite dev server).
 * Captures:
 *   home-dark.png       Home page, dark theme
 *   home-light.png      Home page, light theme
 *   album-dark.png      Album grid page, dark theme
 *   album-light.png     Album grid page, light theme
 *   lightbox-dark.png   Album page with lightbox open, dark theme
 */

import { chromium } from '@playwright/test';
import { mkdir } from 'fs/promises';
import path from 'path';

// --- Parse args -----------------------------------------------------------

const args = process.argv.slice(2);
const get = (flag) => {
	const i = args.indexOf(flag);
	return i !== -1 ? args[i + 1] : null;
};

const baseURL = get('--base-url') ?? 'http://localhost:8080';
const outDir = get('--out') ?? 'screenshots';
const albumArg = get('--album'); // resolved below if not provided
const photoNum = parseInt(get('--photo') ?? '1', 10);

// --- Helpers --------------------------------------------------------------

/**
 * Wait for Svelte hydration (mirrors helpers.ts logic).
 */
async function waitForHydration(page) {
	await page.waitForFunction(() => {
		const v = window.__svelte?.v;
		return v instanceof Set && v.size > 0;
	});
	if (await page.locator('.gallery').count() > 0) {
		await page.locator('.gallery.layout-ready').waitFor();
	}
}

/**
 * Wait for images on the home page (album cover thumbnails).
 */
async function waitForHomeImages(page) {
	// Wait for at least one album cover image to finish loading
	await page.waitForFunction(() => {
		const imgs = document.querySelectorAll('.album-card img');
		return imgs.length > 0 && [...imgs].some((img) => img.complete && img.naturalWidth > 0);
	});
}

/**
 * Wait for grid images on an album page.
 *
 * Images fade in via a 0.4s CSS opacity transition once the `loaded` class is
 * added. We disable that transition before the page loads so images snap to
 * full opacity immediately, then wait for at least 3 to have `loaded`.
 */
async function waitForAlbumImages(page) {
	// Disable the fade-in transition so images are fully opaque as soon as
	// the `loaded` class is applied — no mid-transition screenshot artifacts.
	await page.addStyleTag({ content: '.photo img { transition: none !important; }' });

	await page.waitForFunction(() => {
		return document.querySelectorAll('.photo img.loaded').length >= 3;
	}, null, { timeout: 15000 });
}

/**
 * Apply a theme via addInitScript.
 *
 * addInitScript serializes functions with .toString(), so closure variables are
 * NOT available in the page context. We use the two-argument form instead:
 *   addInitScript(fn, arg) — Playwright serializes arg separately and passes it in.
 *
 * app.html has an inline script that reads localStorage before first paint, so
 * setting it here (before any page script runs) is enough for the theme to apply.
 */
async function applyTheme(page, theme) {
	await page.addInitScript((t) => {
		localStorage.setItem('theme', t);
		// Belt-and-suspenders: also set the attribute directly in case the inline
		// script in app.html has already run by the time theme.ts initializes.
		document.documentElement.setAttribute('data-theme', t);
	}, theme);
}

/**
 * Take a screenshot and log the path.
 */
async function capture(page, filename) {
	const fullPath = path.join(outDir, filename);
	await page.screenshot({ path: fullPath, fullPage: false });
	console.log(`  ✓  ${fullPath}`);
}

// --- Main -----------------------------------------------------------------

async function run() {
	await mkdir(outDir, { recursive: true });

	const browser = await chromium.launch();

	try {
		// Resolve album slug: use --album arg, or grab first album from home page
		let albumSlug = albumArg;
		if (!albumSlug) {
			console.log('Detecting album slug from home page...');
			const ctx = await browser.newContext({ baseURL });
			const page = await ctx.newPage();
			await page.goto('/');
			await waitForHydration(page);
			const href = await page.locator('.album-card').first().getAttribute('href');
			await ctx.close();
			albumSlug = href?.replace('/albums/', '').replace(/\/$/, '') ?? null;
			if (!albumSlug) {
				throw new Error('Could not detect album slug from home page. Use --album <slug>.');
			}
			console.log(`  → using album: ${albumSlug}`);
		}

		const shots = [
			// [theme, route, filename, extraSetup]
			['dark',  '/',                          'home-dark.png',     waitForHomeImages],
			['light', '/',                          'home-light.png',    waitForHomeImages],
			['dark',  `/albums/${albumSlug}`,       'album-dark.png',    waitForAlbumImages],
			['light', `/albums/${albumSlug}`,       'album-light.png',   waitForAlbumImages],
			// Lightbox is handled separately below
		];

		for (const [theme, route, filename, waitFn] of shots) {
			console.log(`Capturing ${filename}...`);
			const ctx = await browser.newContext({ baseURL });
			const page = await ctx.newPage();
			await applyTheme(page, theme);
			await page.goto(route);
			await waitForHydration(page);
			await waitFn(page);
			await capture(page, filename);
			await ctx.close();
		}

		// Lightbox screenshot — dark theme, album page, open photo N
		console.log('Capturing lightbox-dark.png...');
		{
			const ctx = await browser.newContext({ baseURL });
			const page = await ctx.newPage();
			await applyTheme(page, 'dark');
			await page.goto(`/albums/${albumSlug}`);
			await waitForHydration(page);
			await waitForAlbumImages(page);

			// Click the target photo (1-based index from CLI)
			const photoIndex = Math.max(0, photoNum - 1);
			await page.locator('.photo').nth(photoIndex).click();
			await page.locator('.pswp').waitFor({ state: 'visible' });

			// Wait for the full-res image inside PhotoSwipe to load
			await page.waitForFunction(() => {
				const img = document.querySelector('.pswp__img:not(.pswp__img--placeholder)');
				return img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0;
			}, null, { timeout: 15000 });

			await capture(page, 'lightbox-dark.png');
			await ctx.close();
		}

	} finally {
		await browser.close();
	}

	console.log(`\nDone. Screenshots saved to: ${outDir}/`);
}

run().catch((err) => {
	console.error(err);
	process.exit(1);
});
