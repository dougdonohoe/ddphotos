<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import BackToTop from '$lib/components/BackToTop.svelte';
	import OpenGraph from '$lib/components/OpenGraph.svelte';
	import PasswordPrompt from '$lib/components/PasswordPrompt.svelte';
	import type { AlbumSummary } from '$lib/types';
	import {
		SITE_KEY,
		getStoredPassword,
		getAlbumCover,
		storePassword,
		tryDecrypt,
		tryStoredAlbumPasswords
	} from '$lib/crypto';
	import { footerReady } from '$lib/stores';

	let { data } = $props();

	const siteName = import.meta.env.VITE_SITE_NAME;
	const siteUrl = import.meta.env.VITE_SITE_URL;
	const siteDesc = import.meta.env.VITE_SITE_DESCRIPTION;

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
		if (data.encryptedBlob && getStoredPassword(SITE_KEY)) {
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
				const url = getAlbumCover(a.slug);
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
	let ogImage = $derived(ogCover ? `${siteUrl}/albums/${ogCover.coverJpeg}` : undefined);

	onMount(async () => {
		// ?clear removes all stored ddp_* passwords and reloads the page without the param.
		if (new URLSearchParams(window.location.search).has('clear')) {
			try {
				const keys: string[] = [];
				for (let i = 0; i < localStorage.length; i++) {
					const key = localStorage.key(i);
					if (key?.startsWith('ddp_')) keys.push(key);
				}
				keys.forEach((k) => localStorage.removeItem(k));
			} catch {
				// localStorage not available (e.g. private browsing)
			}
			window.location.replace('/');
			return;
		}

		if (!data.encryptedBlob) return;
		unlocking = true;

		// Try the site-wide stored password first.
		const sitePw = getStoredPassword(SITE_KEY);
		if (sitePw) {
			const result = await tryDecrypt(data.encryptedBlob, sitePw);
			if (result) {
				decryptedAlbums = result as AlbumSummary[];
				unlocking = false;
				return;
			}
		}

		// Fall back to any stored per-album password (user may have visited an album first).
		const match = await tryStoredAlbumPasswords(data.encryptedBlob);
		if (match) {
			decryptedAlbums = match.result as AlbumSummary[];
			storePassword(SITE_KEY, match.password);
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
			storePassword(SITE_KEY, password);
		} else {
			shakeCount++;
		}
	}
</script>

<OpenGraph title={siteName} description={siteDesc} url={siteUrl} image={ogImage} />

{#if !data.encryptedBlob}
	<header>
		<h1>{siteName}</h1>
	</header>
{/if}

{#if albums}
	<main class:fade-in={decryptedAlbums !== null}>
		<div class="albums">
			{#each albums as album}
				<a href="/albums/{album.slug}" class="album-card">
					{#if album.cover}
						<img src="/albums/{album.cover}" alt={album.title} />
					{:else if albumCovers[album.slug]}
						<img src={albumCovers[album.slug]} alt={album.title} />
					{:else}
						<div class="album-cover-placeholder" class:loaded={coversLoaded}>
							{#if coversLoaded && album.encrypted}
								<svg
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
							<p class="description">{album.description}</p>
						{/if}
						<p class="meta">{album.count} photos · {album.dateSpan}</p>
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
			title="{siteName} requires a password."
			{shakeCount}
			onunlock={handleUnlock}
		/>
	</div>
{/if}

<style>
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
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}

	/* Background only appears once we've checked localStorage — prevents a dark flash on reload
	   when a cached cover is about to replace the placeholder. */
	.album-cover-placeholder.loaded {
		background: var(--img-placeholder);
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
