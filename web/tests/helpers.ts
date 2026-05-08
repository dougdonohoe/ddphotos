import * as fs from 'fs';
import * as yaml from 'js-yaml';
import { type APIRequestContext, type Page, type Locator } from '@playwright/test';

export interface Passwords {
	all: string | null;
	allHint: string | null;
	albums: Record<string, string>;
	albumHints: Record<string, string>;
}

/**
 * Parse a ddphotos passwords file (sample/config/passwords-*.yaml).
 * Returns { all, albums } from PLAYWRIGHT_PASSWORDS_FILE env var.
 * Returns nulls/empty if the env var is not set (no-password variant).
 */
export function loadPasswords(): Passwords {
	const file = process.env.PLAYWRIGHT_PASSWORDS_FILE;
	if (!file) return { all: null, allHint: null, albums: {}, albumHints: {} };
	const content = fs.readFileSync(file, 'utf-8');
	const parsed = yaml.load(content) as Record<string, unknown>;
	if (!parsed || typeof parsed !== 'object') return { all: null, allHint: null, albums: {}, albumHints: {} };

	const site = parsed['site'] as Record<string, string> | undefined;
	const all: string | null = site?.password ?? null;
	const allHint: string | null = site?.hint ?? null;

	const albums: Record<string, string> = {};
	const albumHints: Record<string, string> = {};
	const albumsMap = parsed['albums'] as Record<string, Record<string, string>> | undefined;
	if (albumsMap) {
		for (const [slug, entry] of Object.entries(albumsMap)) {
			if (entry?.password) albums[slug] = entry.password;
			if (entry?.hint) albumHints[slug] = entry.hint;
		}
	}
	return { all, allHint, albums, albumHints };
}

/**
 * Fill the password prompt and submit. Waits for the albums list to appear.
 * Use for the site-wide password prompt on the home page.
 */
export async function unlockSite(page: Page, password: string): Promise<void> {
	await page.locator('input[type="password"]').fill(password);
	await page.locator('button[type="submit"]').click();
	await page.locator('.albums').waitFor({ timeout: 10_000 });
}

/**
 * Fill the password prompt and submit. Waits for the gallery to be ready.
 * Use for a per-album password prompt on an album page.
 *
 * Waits for .gallery.layout-ready rather than just .gallery: layoutReady is set
 * in onMount (before tryDecryptAlbum), so the class is present as soon as the
 * gallery element appears in the DOM after decryption.
 */
export async function unlockAlbum(page: Page, password: string): Promise<void> {
	await page.locator('input[type="password"]').fill(password);
	await page.locator('button[type="submit"]').click();
	await page.locator('.gallery.layout-ready').waitFor({ timeout: 10_000 });
}

/**
 * Unlock the site if a site-wide password is configured and the prompt is visible.
 * Safe to call unconditionally — no-ops when no password is configured or the page
 * is already unlocked (e.g. password cached in localStorage from a prior unlock).
 *
 * The PasswordPrompt is gated on the Svelte `browser` rune, so it is NOT present in
 * the SSR/static HTML — it only renders after JS hydration. We therefore cannot use
 * isVisible() immediately after goto(); instead we race between the album list
 * appearing (already decrypted) and the overlay appearing (needs unlock).
 */
export async function unlockSiteIfNeeded(page: Page, passwords: Passwords): Promise<void> {
	if (!passwords.all) return;
	const result = await Promise.race([
		page.locator('.albums').waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'unlocked'),
		page.locator('.fullscreen-overlay').waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'locked'),
	]).catch(() => 'timeout');
	if (result !== 'locked') return;
	await unlockSite(page, passwords.all);
}

/**
 * Unlock an album if the prompt is visible and a password is known for the slug.
 * Uses the per-album password if available, otherwise falls back to the site-wide
 * password (which encrypts all albums in the pw-all variant).
 * Safe to call unconditionally — no-ops when no password applies or the album is
 * already decrypted (cached password in localStorage).
 *
 * Same reasoning as unlockSiteIfNeeded: races between content and overlay.
 */
export async function unlockAlbumIfNeeded(
	page: Page,
	slug: string,
	passwords: Passwords
): Promise<void> {
	const pw = passwords.albums[slug] ?? passwords.all;
	if (!pw) return;
	const result = await Promise.race([
		page.locator('.gallery').waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'unlocked'),
		page.locator('.fullscreen-overlay').waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'locked'),
	]).catch(() => 'timeout');
	if (result !== 'locked') return;
	await unlockAlbum(page, pw);
}

/**
 * Return the customCss value from config.json, or null if not set.
 * Fails open (returns null) on API error so tests surface real failures.
 */
export async function siteCustomCss(request: APIRequestContext): Promise<string | null> {
	try {
		const resp = await request.get('/albums/config.json');
		if (!resp.ok()) return null;
		const config = await resp.json();
		return config?.customCss || null;
	} catch {
		return null;
	}
}

/**
 * Check whether a specific album slug exists in albums.json.
 * Fails open (returns true) on API error so tests surface real failures.
 */
export async function albumExists(request: APIRequestContext, slug: string): Promise<boolean> {
	try {
		const resp = await request.get('/albums/albums.json');
		if (!resp.ok()) return true;
		const albums: { slug: string }[] = await resp.json();
		return albums.some((a) => a.slug === slug);
	} catch {
		return true;
	}
}

/**
 * Check whether the site has at least n albums that contain photos (count > 0).
 * Fails open (returns true) on API error.
 */
export async function hasAtLeastNAlbums(request: APIRequestContext, n: number): Promise<boolean> {
	try {
		const resp = await request.get('/albums/albums.json');
		if (!resp.ok()) return true;
		const albums: { count: number }[] = await resp.json();
		return albums.filter((a) => a.count > 0).length >= n;
	} catch {
		return true;
	}
}

/**
 * Extract the album slug from a card locator's href attribute.
 */
export async function slugFromCard(card: Locator): Promise<string> {
	const href = (await card.getAttribute('href')) ?? '';
	return href.replace('/albums/', '');
}

/**
 * Wait for Svelte 5 to finish hydrating the page.
 *
 * In production (Apache, bundled JS) Svelte hydrates synchronously during
 * page load, so tests can click immediately. In the Vite dev server, modules
 * are loaded via dynamic import chains and hydration completes ~200ms after
 * the load event — before which onclick handlers are not yet attached.
 *
 * Svelte 5 registers each mounted component in window.__svelte.v (a Set).
 * Waiting for a non-zero size is an initial signal that the app is hydrating.
 *
 * On album pages, we also wait for .gallery.layout-ready — a flag set in the
 * album page component's onMount (see +page.svelte). This is needed because
 * SvelteKit mounts the layout component first (satisfying the __svelte.v check)
 * and then the page component. On a cold Vite dev server the gap between the
 * two is enough for a click to land before onclick handlers are attached.
 */
export async function waitForHydration(page: Page): Promise<void> {
	await page.waitForFunction(() => {
		const v = (window as any).__svelte?.v;
		return v instanceof Set && v.size > 0;
	});

	// If this page has a photo gallery, wait for the album page component's
	// onMount to signal readiness via the layout-ready class. .gallery is only
	// in the DOM when the album is decrypted (it lives inside {#if album}), so
	// count() correctly skips this wait on locked album pages.
	if ((await page.locator('.gallery').count()) > 0) {
		await page.locator('.gallery.layout-ready').waitFor();
	}
}
