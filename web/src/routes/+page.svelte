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
	import type { AlbumSummary, SiteHtmlContent } from '$lib/types';
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
	import Lock from 'lucide-svelte/icons/lock';
	import Image from 'lucide-svelte/icons/image';
	import CameraOff from 'lucide-svelte/icons/camera-off';

	let { data } = $props();

	// Unpack siteData into flat reactive locals so the rest of this component reads cleanly.
	const siteId = $derived(data.siteData.siteId);
	const albumsEncrypted = $derived(data.siteData.albums.encrypted);
	const encryptedAlbumsBlob = $derived(data.siteData.albums.encrypted ? data.siteData.albums.blob : null);
	const loadedAlbums = $derived(data.siteData.albums.encrypted ? null : data.siteData.albums.data);
	const siteHint = $derived(data.siteData.albums.encrypted ? data.siteData.albums.hint : undefined);
	const _html = $derived(data.siteData.html);
	const encryptedHtmlBlob = $derived(_html !== null && _html.encrypted ? _html.blob : null);
	const staticHtml = $derived(_html !== null && !_html.encrypted ? _html.data : null);

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

	// HTML content from html.json or html.enc.json; null until decrypted (encrypted sites).
	let decryptedSiteHtml = $state<SiteHtmlContent | null>(null);
	// Effective HTML content: decrypted takes precedence over statically loaded (unencrypted sites).
	const effectiveSiteHtml = $derived(decryptedSiteHtml ?? staticHtml);
	const siteTitleHtml = $derived(effectiveSiteHtml?.siteTitleHtml ?? siteName);
	const siteSubtitleHtml = $derived(effectiveSiteHtml?.siteSubtitleHtml);
	const siteOverviewHtml = $derived(effectiveSiteHtml?.siteOverviewHtml);

	// Client-decrypted albums list (null until decryption succeeds).
	let decryptedAlbums = $state<AlbumSummary[] | null>(null);
	// Effective list: server-provided (unencrypted) takes precedence, else client-decrypted.
	let albums = $derived(loadedAlbums ?? decryptedAlbums);
	// True while we're silently trying stored passwords so we don't flash the prompt.
	// $effect.pre runs synchronously before Svelte's first DOM commit in the browser,
	// so if a stored password exists we set unlocking=true before the prompt ever renders.
	// (On SSR, effects don't run; unlocking stays false and the prompt renders in the
	// static HTML — this is fine since JS will correct it immediately on hydration.)
	let unlocking = $state(false);
	$effect.pre(() => {
		if (albumsEncrypted && getStoredPassword(siteKey(siteId))) {
			unlocking = true;
		}
	});
	// Hide footer until albums are ready on encrypted sites, preventing a layout jump.
	$effect.pre(() => {
		if (albumsEncrypted) {
			footerReady.set(albums !== null);
		}
	});
	let shakeCount = $state(0);

	// Cover URLs for per-album encrypted albums, loaded from localStorage.
	//
	// We initialize synchronously from localStorage when loadedAlbums is already available
	// (per-album encrypted, non-site-encrypted pages). This means albumCovers is populated
	// before the first render on the client, so the img element is in the DOM from the start
	// and the browser can display it without an intermediate placeholder flash.
	//
	// coversLoaded stays false during SSR (effects don't run server-side), preventing the
	// lock icon SVG from being baked into the static HTML. On the client it's true immediately
	// (when loadedAlbums is available) or set by $effect.pre after decryption (site-encrypted).
	function readStoredCovers(albumList: AlbumSummary[]): Record<string, string> {
		const covers: Record<string, string> = {};
		for (const a of albumList) {
			if (a.encrypted && !a.cover) {
				const url = getAlbumCover(siteId, a.slug);
				if (url) covers[a.slug] = url;
			}
		}
		return covers;
	}
	// Capture once — loadedAlbums is static (set at load time, never changes at runtime).
	// untrack tells Svelte we intentionally want the initial value, not a reactive binding.
	const initialAlbums = untrack(() => loadedAlbums);
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
		syncSiteId(siteId, data.siteConfig?.keyId);

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

		if (!albumsEncrypted) return;
		unlocking = true;

		const sitePw = getStoredPassword(siteKey(siteId));
		if (sitePw && await applyDecrypted(sitePw)) {
			unlocking = false;
			return;
		}

		// Fall back to any stored per-album password (user may have visited an album first).
		const match = await tryStoredAlbumPasswords(encryptedAlbumsBlob!, siteId);
		if (match) {
			await applyDecrypted(match.password);
			storePassword(siteKey(siteId), match.password);
			unlocking = false;
			return;
		}

		unlocking = false;
	});

	// Decrypt both album and html blobs with the given password.
	// Applies results to reactive state and returns true on success.
	async function applyDecrypted(password: string): Promise<boolean> {
		const [albumResult, htmlResult] = await Promise.all([
			tryDecrypt(encryptedAlbumsBlob!, password),
			encryptedHtmlBlob ? tryDecrypt(encryptedHtmlBlob, password) : Promise.resolve(null)
		]);
		if (!albumResult) return false;
		// Set siteHtml before albums so both are ready in the same Svelte DOM commit.
		if (htmlResult) decryptedSiteHtml = htmlResult as SiteHtmlContent;
		decryptedAlbums = albumResult as AlbumSummary[];
		return true;
	}

	async function handleUnlock(password: string) {
		if (!albumsEncrypted) return;
		if (await applyDecrypted(password)) {
			storePassword(siteKey(siteId), password);
		} else {
			shakeCount++;
		}
	}
</script>

<OpenGraph title={siteName} description={siteDesc} url={siteUrl} {siteName} image={ogImage} />

{#if !albumsEncrypted || albums}
	{#if data.siteConfig?.heroImage}
		<div class="hero">
			<img src="/albums/{data.siteConfig.heroImage}" alt={siteName} />
			<div class="hero-overlay">
				<h1>{@html siteTitleHtml}</h1>
				{#if siteSubtitleHtml}
					<p class="site-subtitle">{@html siteSubtitleHtml}</p>
				{/if}
			</div>
		</div>
	{:else}
		<header>
			<h1>{@html siteTitleHtml}</h1>
			{#if siteSubtitleHtml}
				<p class="site-subtitle">{@html siteSubtitleHtml}</p>
			{/if}
		</header>
	{/if}
{/if}

{#if albums}
	<main>
		{#if siteOverviewHtml}
			<div class="site-overview">{@html siteOverviewHtml}</div>
		{/if}
		<div class="albums">
			{#each albums as album (album.slug)}
				<a href={resolve(`/albums/${album.slug}`)} class="album-card"
					onkeydown={(e) => { if (e.key === ' ') { e.preventDefault(); e.currentTarget.click(); } }}
				>
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
								<CameraOff size={48} strokeWidth={1.5} aria-hidden="true" />
							{:else if album.encrypted}
								<!-- Always in SSR HTML; --lock-vis hides it when a cached cover shows instead -->
								<Lock class="ddp-lock-icon" size={72} strokeWidth={1.5} aria-hidden="true" />
							{:else if coversLoaded}
								<Image size={36} strokeWidth={1.5} aria-hidden="true" />
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

{#if browser && albumsEncrypted && !albums && !unlocking}
	<div class="fullscreen-overlay">
		<PasswordPrompt
			name={siteName}
			hint={siteHint}
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

	.hero-overlay h1 :global(a) {
		color: inherit;
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 5px;
	}

	.hero-overlay h1 :global(a:hover) {
		opacity: 0.85;
	}

	.hero-overlay .site-subtitle {
		margin: 0.4rem 0 0;
		font-size: 1.15rem;
		font-style: italic;
		color: rgba(255, 255, 255, 0.88);
		text-shadow: 0 1px 3px rgba(0, 0, 0, 0.5);
	}

	@media (max-width: 480px) {
		.hero {
			height: 140px;
		}

		.hero-overlay {
			padding-bottom: 1.0rem;
		}

		.hero-overlay h1 {
			font-size: 1.7rem;
		}

		.hero-overlay .site-subtitle {
			font-size: 0.95rem;
		}
	}

	header {
		padding: 1rem 2rem 0;
	}

	h1 {
		margin: 0;
		font-size: 2.4rem;
	}

	header h1 :global(a) {
		color: inherit;
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
	}

	header h1 :global(a:hover) {
		opacity: 0.85;
	}

	header .site-subtitle {
		margin: 0.4rem 0 0;
		font-size: 1.15rem;
		font-style: italic;
		color: var(--text-muted);
	}

	@media (max-width: 480px) {
		h1 {
			font-size: 1.7rem;
		}

		header .site-subtitle {
			font-size: 0.95rem;
		}
	}

	.site-overview {
		font-size: 1.1rem;
		color: var(--text-color);
		margin-bottom: 1.5rem;
		line-height: 1.2;
	}

	.site-overview :global(a) {
		color: var(--link-color);
		text-decoration: none;
	}

	.site-overview :global(a:hover) {
		text-decoration: underline;
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
	:global(.ddp-lock-icon) {
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
</style>
