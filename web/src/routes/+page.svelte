<script module>
	// Persists across home-page component remounts (navigating away and back).
	// Set by beforeNavigate when leaving home; consumed by onMount + afterNavigate on return.
	let savedScrollY = 0;
</script>

<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import { beforeNavigate, afterNavigate, disableScrollHandling } from '$app/navigation';
	import { resolve } from '$app/paths';
	import BackToTop from '$lib/components/BackToTop.svelte';
	import OpenGraph from '$lib/components/OpenGraph.svelte';
	import PasswordPrompt from '$lib/components/PasswordPrompt.svelte';
	import type { AlbumSummary } from '$lib/types';
	import {
		siteKey,
		getStoredPassword,
		getAlbumCover,
		storePassword,
		syncSiteId,
		tryDecrypt,
		tryStoredAlbumPasswords
	} from '$lib/crypto';
	import { footerReady } from '$lib/stores';

	let { data } = $props();

	// ── Scroll restoration ──────────────────────────────────────────────────────────────
	// Problem: when navigating back to home from an album, the albums grid renders
	// asynchronously *after* afterNavigate fires (SvelteKit's commit_promise + two tick()s
	// don't wait for full Svelte 5 rendering). scrollTo during afterNavigate always hits a
	// page that's too short (scrollHeight = viewportHeight), so it clamps to 0.
	//
	// Solution: use a reactive $effect that watches `albums`. It fires as soon as Svelte
	// finishes rendering the grid (i.e. when albums becomes non-null). At that point the
	// page is tall enough to scroll. The page is hidden during the wait so there's no flash.
	//
	// savedScrollY  — module-level, survives component remounts between navigations
	// pendingScroll — component-level $state, triggers the $effect, cleared after scroll
	let pendingScroll = $state(0);

	beforeNavigate(() => {
		savedScrollY = window.scrollY;
	});

	afterNavigate(({ from }) => {
		const y = savedScrollY;
		savedScrollY = 0;
		if (y > 0 && from?.url.pathname.startsWith('/albums/')) {
			pendingScroll = y; // $effect below will scroll once albums renders
		} else {
			// Not returning from album — unhide (set in onMount) and optionally reset scroll
			requestAnimationFrame(() => {
				document.documentElement.style.visibility = '';
				if (y > 0) window.scrollTo(0, 0);
			});
		}
	});

	// Fire whenever albums becomes non-null (grid rendered) AND we have a pending scroll.
	// $effect runs after Svelte commits DOM updates, so the grid is in the DOM by this point.
	$effect(() => {
		if (albums && pendingScroll > 0) {
			const y = pendingScroll;
			untrack(() => { pendingScroll = 0; }); // clear without re-triggering this effect
			requestAnimationFrame(() => {
				document.documentElement.style.visibility = '';
				window.scrollTo(0, y);
			});
		}
	});

	const siteName = $derived(data.siteConfig.siteName);
	const siteUrl = $derived(data.siteConfig.siteUrl);
	const siteDesc = $derived(data.siteConfig.siteDescription);

	// Client-decrypted albums list (null until decryption succeeds).
	let decryptedAlbums = $state<AlbumSummary[] | null>(null);
	// Effective list: server-provided (unencrypted) takes precedence, else client-decrypted.
	let albums = $derived(data.albums ?? decryptedAlbums);
	// True while we're silently trying stored passwords so we don't flash the prompt.
	// $effect.pre runs synchronously before Svelte's first DOM commit in the browser,
	// so if a stored password exists we set unlocking=true before the prompt ever renders.
	// (On SSR, effects don't run; unlocking stays false and the prompt renders in the
	// static HTML — this is fine since JS will correct it immediately on hydration.)
	let unlocking = $state(false);
	$effect.pre(() => {
		if (data.encryptedBlob && getStoredPassword(siteKey(data.siteId))) {
			unlocking = true;
		}
	});
	// Hide footer until albums are ready on encrypted sites, preventing a layout jump.
	$effect.pre(() => {
		if (data.encryptedBlob) {
			footerReady.set(albums !== null);
		}
	});
	let shakeCount = $state(0);

	// Cover URLs for per-album encrypted albums, loaded from localStorage.
	//
	// We initialize synchronously from localStorage when data.albums is already available
	// (per-album encrypted, non-site-encrypted pages). This means albumCovers is populated
	// before the first render on the client, so the img element is in the DOM from the start
	// and the browser can display it without an intermediate placeholder flash.
	//
	// coversLoaded stays false during SSR (effects don't run server-side), preventing the
	// lock icon SVG from being baked into the static HTML. On the client it's true immediately
	// (when data.albums is available) or set by $effect.pre after decryption (site-encrypted).
	function readStoredCovers(albumList: AlbumSummary[]): Record<string, string> {
		const covers: Record<string, string> = {};
		for (const a of albumList) {
			if (a.encrypted && !a.cover) {
				const url = getAlbumCover(data.siteId, a.slug);
				if (url) covers[a.slug] = url;
			}
		}
		return covers;
	}
	// Capture once — data.albums is static (set at load time, never changes at runtime).
	// untrack tells Svelte we intentionally want the initial value, not a reactive binding.
	const initialAlbums = untrack(() => data.albums);
	let albumCovers = $state<Record<string, string>>(
		browser && initialAlbums ? readStoredCovers(initialAlbums) : {}
	);
	let coversLoaded = $state(browser && initialAlbums !== null);
	// For site-encrypted pages, albums arrive later (after decryption). Re-read covers then.
	$effect.pre(() => {
		if (!albums || initialAlbums !== null) return;
		albumCovers = readStoredCovers(albums);
		coversLoaded = true;
	});

	let ogCover = $derived(albums?.find((a) => !a.encrypted && a.coverJpeg));
	let ogImage = $derived(
		data.siteConfig?.heroImage
			? `${siteUrl}/albums/${data.siteConfig.heroImage}`
			: ogCover
				? `${siteUrl}/albums/${ogCover.coverJpeg}`
				: undefined
	);

	onMount(async () => {
		// onMount fires during SvelteKit's tick() calls, before scroll handling in navigate().
		// When we have a saved scroll position to restore:
		// - disableScrollHandling() prevents SvelteKit from resetting scroll to 0
		// - visibility:hidden hides the page so the user doesn't see a flash at position 0
		//   while we wait for layout to be computed (afterNavigate rAF unhides it)
		if (savedScrollY > 0) {
			disableScrollHandling();
			document.documentElement.style.visibility = 'hidden';
			// Failsafe: if $effect never fires (e.g. albums stays null due to error), unhide.
			setTimeout(() => { document.documentElement.style.visibility = ''; }, 2000);
		}


		// Clear stale cover cache if the siteId or keyId changed (key rotation renames all image files).
		syncSiteId(data.siteId, data.siteConfig?.keyId);

		// ?clear removes all stored ddp_* passwords and reloads the page without the param.
		if (new URLSearchParams(window.location.search).has('clear')) {
			try {
				const keys: string[] = [];
				for (let i = 0; i < localStorage.length; i++) {
					const key = localStorage.key(i);
					if (key?.startsWith('ddp_')) keys.push(key);
				}
				keys.forEach((k) => localStorage.removeItem(k));
				localStorage.removeItem('theme');
			} catch {
				// localStorage not available (e.g. private browsing)
			}
			window.location.replace('/');
			return;
		}

		if (!data.encryptedBlob) return;
		unlocking = true;

		// Try the site-wide stored password first.
		const sitePw = getStoredPassword(siteKey(data.siteId));
		if (sitePw) {
			const result = await tryDecrypt(data.encryptedBlob, sitePw);
			if (result) {
				decryptedAlbums = result as AlbumSummary[];
				unlocking = false;
				return;
			}
		}

		// Fall back to any stored per-album password (user may have visited an album first).
		const match = await tryStoredAlbumPasswords(data.encryptedBlob, data.siteId);
		if (match) {
			decryptedAlbums = match.result as AlbumSummary[];
			storePassword(siteKey(data.siteId), match.password);
			unlocking = false;
			return;
		}

		unlocking = false;
	});

	async function handleUnlock(password: string) {
		if (!data.encryptedBlob) return;
		const result = await tryDecrypt(data.encryptedBlob, password);
		if (result) {
			decryptedAlbums = result as AlbumSummary[];
			storePassword(siteKey(data.siteId), password);
		} else {
			shakeCount++;
		}
	}
</script>

<OpenGraph title={siteName} description={siteDesc} url={siteUrl} {siteName} image={ogImage} />

{#if !data.encryptedBlob || albums}
	{#if data.siteConfig?.heroImage}
		<div class="hero">
			<img src="/albums/{data.siteConfig.heroImage}" alt={siteName} />
			<div class="hero-overlay">
				<h1>{siteName}</h1>
			</div>
		</div>
	{:else}
		<header>
			<h1>{siteName}</h1>
		</header>
	{/if}
{/if}

{#if albums}
	<main class:fade-in={decryptedAlbums !== null}>
		<div class="albums">
			{#each albums as album (album.slug)}
				<a href={resolve(`/albums/${album.slug}`)} class="album-card">
					{#if album.cover}
						<img src="/albums/{album.cover}" alt={album.title} />
					{:else}
						<!-- For per-album encrypted albums the cover URL lives in localStorage.
						     The inline <head> script sets --ddp-cover-SLUG on <html> before first
						     paint, so this div already shows the cover on SSR. After hydration,
						     albumCovers[slug] is set and the explicit url() replaces the var().
						     Using background-image (not <img>) avoids any DOM swap flash. -->
						<div
							class="album-cover-placeholder"
							style:background-image={albumCovers[album.slug]
								? `url('${albumCovers[album.slug]}')`
								: `var(--ddp-cover-${album.slug}, none)`}
							style:background-size="cover"
							style:background-position="center"
							style:--lock-vis={albumCovers[album.slug]
								? 'hidden'
								: `var(--ddp-icon-vis-${album.slug}, visible)`}
						>
							{#if album.count === 0}
								<svg
									xmlns="http://www.w3.org/2000/svg"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="1.5"
									stroke-linecap="round"
									stroke-linejoin="round"
									width="48"
									height="48"
									aria-hidden="true"
								>
									<rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
									<line x1="3" y1="3" x2="21" y2="21"></line>
								</svg>
							{:else if album.encrypted}
								<!-- Always in SSR HTML; --lock-vis hides it when a cached cover shows instead -->
								<svg
									class="ddp-lock-icon"
									xmlns="http://www.w3.org/2000/svg"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="1.5"
									stroke-linecap="round"
									stroke-linejoin="round"
									width="72"
									height="72"
									aria-hidden="true"
								>
									<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
									<path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
								</svg>
							{:else if coversLoaded}
								<svg
									xmlns="http://www.w3.org/2000/svg"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="1.5"
									stroke-linecap="round"
									stroke-linejoin="round"
									width="36"
									height="36"
									aria-hidden="true"
								>
									<rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
									<circle cx="8.5" cy="8.5" r="1.5"></circle>
									<polyline points="21 15 16 10 5 21"></polyline>
								</svg>
							{/if}
						</div>
					{/if}
					<div class="album-info">
						<h2>{album.title}</h2>
						{#if album.description}
							<p class="description">{@html album.description}</p>
						{/if}
						<p class="meta">{album.count} photos{album.dateSpan ? ` · ${album.dateSpan}` : ''}</p>
					</div>
				</a>
			{/each}
		</div>

		<BackToTop mobileOnly={true} />
	</main>
{:else}
	<main class="loading-page"></main>
{/if}

{#if browser && data.encryptedBlob && !albums && !unlocking}
	<div class="fullscreen-overlay">
		<PasswordPrompt
			name={siteName}
			hint={data.siteHint}
			{shakeCount}
			onunlock={handleUnlock}
		/>
	</div>
{/if}

<style>
	.hero {
		position: relative;
		width: 100%;
		height: 250px;
		overflow: hidden;
	}

	.hero img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	.hero-overlay {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1.5rem 2rem;
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.65));
	}

	.hero-overlay h1 {
		margin: 0;
		font-size: 2.8rem;
		color: #fff;
		text-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
	}

	@media (max-width: 480px) {
		.hero {
			height: 140px;
		}

		.hero-overlay h1 {
			font-size: 1.7rem;
		}
	}

	header {
		padding: 1rem 2rem 0;
	}

	h1 {
		margin: 0;
		font-size: 2.4rem;
	}

	@media (max-width: 480px) {
		h1 {
			font-size: 1.7rem;
		}
	}

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem 2rem 2rem;
	}

	.loading-page {
		min-height: 80vh;
	}

	.albums {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 1.5rem;
	}

	.album-card {
		text-decoration: none;
		color: inherit;
		border-radius: 8px;
		overflow: hidden;
		background: var(--bg-secondary);
		box-shadow: 0 2px 8px var(--shadow-color);
		transition: transform 0.2s, box-shadow 0.2s;
		display: flex;
		flex-direction: column;
	}

	.album-card:hover {
		transform: translateY(-4px);
		box-shadow: 0 4px 16px var(--shadow-color);
	}

	.album-card img {
		width: 100%;
		aspect-ratio: 3 / 2;
		object-fit: cover;
		background: var(--bg-secondary);
	}

	.album-cover-placeholder {
		width: 100%;
		aspect-ratio: 3 / 2;
		background: var(--img-placeholder);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}

	/* Lock icon visibility is controlled by --lock-vis, set on the parent div.
	   The inline <head> script sets --ddp-icon-vis-SLUG: hidden when a cover is cached,
	   so the icon is hidden from the very first paint when a cover will be shown instead. */
	.ddp-lock-icon {
		visibility: var(--lock-vis, visible);
	}

	.album-info {
		padding: 1rem 1rem 0.5rem;
		display: flex;
		flex-direction: column;
		flex: 1;
	}

	.album-info h2 {
		margin: 0 0 0.5rem 0;
		font-size: 1.25rem;
	}

	.album-info p {
		margin: 0;
		color: var(--text-muted);
		font-size: 0.9rem;
	}

	.album-info .meta {
		margin-top: auto;
		padding-top: 0.4rem;
		text-align: right;
		font-style: italic;
		font-size: 0.85rem;
	}

	.album-info .description {
		margin-top: 0.3rem;
		font-size: 0.95rem;
		color: var(--text-color-2nd);
		opacity: 0.8;
	}

	/* Full-screen overlay covering header/footer when site is locked */
	.fullscreen-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-color);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	/* Fade in real albums after decryption */
	@keyframes fade-in {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	main.fade-in {
		animation: fade-in 400ms ease-out forwards;
	}
</style>
