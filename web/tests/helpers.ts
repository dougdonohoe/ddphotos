import { type Page } from '@playwright/test';

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
	// onMount to signal readiness via the layout-ready class. The .gallery
	// element is present in the prerendered HTML so count() is always reliable.
	if (await page.locator('.gallery').count() > 0) {
		await page.locator('.gallery.layout-ready').waitFor();
	}
}
