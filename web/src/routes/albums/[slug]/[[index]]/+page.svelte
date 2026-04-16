<!-- Suppress IntelliJ's "Selector is never used" warning for :global(.pswp) and similar
     selectors that target DOM elements injected dynamically by third-party libraries
     (PhotoSwipe). IntelliJ's static CSS analyzer can't see runtime-injected classes. -->
<!--suppress CssUnusedSymbol -->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { browser } from '$app/environment';
	import { goto, replaceState, pushState } from '$app/navigation';
	import justifiedLayout from 'justified-layout';
	import PhotoSwipe from 'photoswipe';
	import 'photoswipe/style.css';
	import BackToTop from '$lib/components/BackToTop.svelte';
	import OpenGraph from '$lib/components/OpenGraph.svelte';
	import PasswordPrompt from '$lib/components/PasswordPrompt.svelte';
	import type { AlbumIndex } from '$lib/types';
	import {
		siteKey,
		albumKey,
		getStoredPassword,
		storePassword,
		storeAlbumCover,
		syncSiteId,
		tryDecrypt
	} from '$lib/crypto';
	import { footerReady } from '$lib/stores';

	let { data } = $props();

	// Client-decrypted album (null until the user's stored password or manual entry works).
	let decryptedAlbum = $state<AlbumIndex | null>(null);
	// Effective album: server-provided (unencrypted) takes precedence, else client-decrypted.
	let album = $derived(data.album ?? decryptedAlbum);
	// Metadata: prefer server-loaded values; fall back to fields embedded in the decrypted index
	// (needed for site-encrypted sites where albums.enc.json is not fetched server-side).
	let albumTitle = $derived(album?.title ?? data.albumTitle);
	let description = $derived(data.description || album?.description || '');
	let dateSpan = $derived(data.dateSpan || album?.dateSpan || '');
	// True while we're silently trying stored passwords so we don't flash the prompt.
	// $effect.pre runs synchronously before Svelte's first DOM commit in the browser,
	// so if a stored password exists we set unlocking=true before the prompt ever renders.
	// (On SSR, effects don't run; unlocking stays false and the prompt renders in the
	// static HTML — this is fine since JS will correct it immediately on hydration.)
	let unlocking = $state(false);
	$effect.pre(() => {
		if (data.encryptedBlob && (getStoredPassword(albumKey(data.siteId, data.slug)) || getStoredPassword(siteKey(data.siteId)))) {
			unlocking = true;
		}
	});
	// Hide footer until album is ready on encrypted pages, preventing a layout jump.
	$effect.pre(() => {
		if (data.encryptedBlob) {
			footerReady.set(album !== null);
		}
	});
	let shakeCount = $state(0);

	let containerWidth = $state(1200);
	let container = $state<HTMLDivElement | undefined>(undefined);
	// Re-measure when container is bound — handles the encrypted case where the gallery div
	// isn't in the DOM at onMount time (album is null), so onMount's updateWidth() is a no-op.
	$effect(() => {
		if (container) containerWidth = container.clientWidth;
	});
	let lightboxOpen = $state(false);
	let lightboxClosedAt = $state(0);
	let pswpInstance: PhotoSwipe | null = null; // reference to the open PhotoSwipe instance
	// Stored so onMount cleanup can remove it when navigating away via a link while the
	// lightbox is open (component unmounts before the close event fires).
	let activePopstateHandler: (() => void) | null = null;
	// Scroll target set as user navigates in the lightbox; applied on close.
	let pendingScrollY: number | null = null;
	// Photo index to focus in the grid when the lightbox closes.
	let pendingFocusIndex: number | null = null;

	// Image fade-in state. Populated by the $effect below, which re-runs on album change.
	let imageSrcs = $state<string[]>([]); // src per image; empty string = not yet assigned
	let imageLoaded = $state<boolean[]>([]); // true once the browser fires the load event
	let slowMode = $state(browser && new URLSearchParams(window.location.search).has('slow')); // true when ?slow is in the URL
	let layoutReady = $state(false); // true after onMount measures the real container width
	let lastEffectSlug = '';          // non-reactive: tracks which slug $effect last reset imageLoaded for
	let lastEffectPhotosLen = 0;     // non-reactive: tracks last known photo count to detect album load
	// 1-based photo number for display when the route index is out of range; null otherwise.
	let invalidPhotoIndex = $derived(
		album !== null &&
			data.photoIndex !== null &&
			(data.photoIndex < 0 || data.photoIndex >= album.photos.length)
			? data.photoIndex + 1
			: null
	);

	// Compute layout based on photo aspect ratios
	let layout = $derived(() => {
		const aspectRatios = (album?.photos ?? []).map((p) => p.width / p.height);
		return justifiedLayout(aspectRatios, {
			containerWidth,
			targetRowHeight: 300,
			containerPadding: 0,
			boxSpacing: 8
		});
	});

	// Build PhotoSwipe data source
	let photoswipeItems = $derived(
		(album?.photos ?? []).map((photo) => ({
			src: `/albums/${data.slug}/${photo.src.full}`,
			w: photo.width,
			h: photo.height,
			msrc: `/albums/${data.slug}/${photo.src.grid}`, // thumbnail for loading
			alt: photo.description || photo.fileName,
			caption: photo.description || ''
		}))
	);

	async function tryDecryptAlbum() {
		if (!data.encryptedBlob) return;
		unlocking = true;

		// Try per-album stored password first, then site-wide.
		for (const key of [albumKey(data.siteId, data.slug), siteKey(data.siteId)]) {
			const pw = getStoredPassword(key);
			if (pw) {
				const result = await tryDecrypt(data.encryptedBlob, pw);
				if (result) {
					decryptedAlbum = result as AlbumIndex;
					storePassword(albumKey(data.siteId, data.slug), pw);
					const cover = decryptedAlbum.cover ?? decryptedAlbum.photos[0]?.src.grid;
					if (cover) storeAlbumCover(data.siteId, data.slug, `/albums/${data.slug}/${cover}`);
					unlocking = false;
					// Wait for Svelte to recompute photoswipeItems from the decrypted album,
					// then auto-open the lightbox if this is a permalink URL.
					await tick();
					if (data.photoIndex !== null && invalidPhotoIndex === null) {
						openLightbox(data.photoIndex, false);
					}
					return;
				}
			}
		}

		unlocking = false;
	}

	async function handleUnlock(password: string) {
		if (!data.encryptedBlob) return;
		const result = await tryDecrypt(data.encryptedBlob, password);
		if (result) {
			decryptedAlbum = result as AlbumIndex;
			storePassword(albumKey(data.siteId, data.slug), password);
			const cover = decryptedAlbum.cover ?? decryptedAlbum.photos[0]?.src.grid;
			if (cover) storeAlbumCover(data.siteId, data.slug, `/albums/${data.slug}/${cover}`);
		} else {
			shakeCount++;
		}
	}

	function openLightbox(index: number, animate = true) {
		pendingFocusIndex = index;
		const pswp = new PhotoSwipe({
			dataSource: photoswipeItems,
			index,
			bgClickAction: 'close',
			closeOnVerticalDrag: true,
			padding: { top: 0, bottom: 0, left: 0, right: 0 },
			showAnimationDuration: animate ? undefined : 0
		});

		// Whether back-button navigation triggered this close (set by handlePopstate).
		let closedByBackNav = false;
		// Whether we pushed a native history entry when opening (animate=true case).
		// Determines how to restore the URL when the lightbox closes normally.
		let pushedHistoryEntry = false;

		// Listen for browser back/forward while the lightbox is open.
		//
		// We use a native popstate listener rather than SvelteKit's beforeNavigate because
		// the history entry we push below is a *native* entry (no SvelteKit session key).
		// SvelteKit's own popstate handler checks for its session key and returns early when
		// it finds none, so it never fires beforeNavigate for our entry — leaving this
		// listener as the sole handler for back-nav-while-lightbox-is-open.
		const handlePopstate = () => {
			closedByBackNav = true;
			pswpInstance = null; // null before close() so onMount cleanup skips destroy()
			pswp.close(); // plays close animation; close handler cleans up the rest
		};
		window.addEventListener('popstate', handlePopstate);
		activePopstateHandler = handlePopstate;

		// Request fullscreen on mobile for immersive viewing
		pswp.on('openingAnimationEnd', () => {
			if (document.documentElement.requestFullscreen && window.innerWidth <= 768) {
				document.documentElement.requestFullscreen().catch(() => {
					// Fullscreen request failed (user denied or not supported)
				});
			}
		});

		// Exit fullscreen when closing lightbox
		pswp.on('close', () => {
			if (document.fullscreenElement) {
				document.exitFullscreen().catch(() => {});
			}
		});

		pswp.on('openingAnimationStart', () => {
			lightboxOpen = true;
		});
		pswp.on('close', () => {
			window.removeEventListener('popstate', handlePopstate);
			activePopstateHandler = null;
			pswpInstance = null;
			lightboxOpen = false;
			lightboxClosedAt = Date.now();
			if (!closedByBackNav) {
				const target = pendingScrollY !== null ? Math.max(0, pendingScrollY) : null;
				pendingScrollY = null;

				if (pushedHistoryEntry) {
					history.go(-1);
				} else {
					replaceState(`/albums/${data.slug}`, {});
				}

				const focusIdx = pendingFocusIndex;
				pendingFocusIndex = null;
				// Cache the button to focus (queried once, before the guard loop starts).
				const focusBtn = (focusIdx !== null && container)
					? (container.querySelectorAll<HTMLElement>('.photo')[focusIdx] ?? null)
					: null;

				// Run a guard loop for 1000ms to fight both PhotoSwipe's built-in focus
				// restoration (fires first frame) and SvelteKit's async focus reset (fires
				// ~300-500ms in, same as its scroll reset). Re-apply only when focus has
				// moved elsewhere to avoid redundant focus events.
				// Also re-applies scroll if SvelteKit resets it (same as before).
				const deadline = performance.now() + 1000;
				const guard = () => {
					if (target !== null && Math.abs(window.scrollY - target) > 1) {
						window.scrollTo({ top: target, behavior: 'instant' });
					}
					if (focusBtn && document.activeElement !== focusBtn) {
						focusBtn.focus({ preventScroll: true });
					}
					if (performance.now() < deadline) requestAnimationFrame(guard);
				};
				requestAnimationFrame(guard);
			}
			// closedByBackNav: back button already navigated to the correct URL; nothing to do.
		});

		pswp.init();
		pswpInstance = pswp;

		// Push a history entry when opening so back returns to /albums/slug rather than
		// to whatever page preceded the album.  Uses SvelteKit's pushState (not native
		// history.pushState) to keep SvelteKit's router in sync and suppress the console
		// warning it emits for direct history mutations.
		//
		// NOTE: SvelteKit's popstate handler ALSO fires for this entry (it recognizes its
		// own session key), so both SvelteKit and our handlePopstate run on back-nav.
		// SvelteKit navigates to /albums/slug; handlePopstate closes the lightbox — the
		// two are independent and don't conflict.
		//
		// Skip for animate=false (permalink open): URL already has the photo index.
		if (animate) {
			pushState(`/albums/${data.slug}/${index + 1}`, {});
			pushedHistoryEntry = true;
		}
		pswp.on('change', () => {
			// SvelteKit's replaceState keeps the photo URL current as the user navigates.
			// Uses replaceState (not pushState) so every photo doesn't add a history entry
			// — back always jumps directly to the album rather than stepping photo-by-photo.
			replaceState(`/albums/${data.slug}/${pswp.currIndex + 1}`, {});
			// Store the target scroll so the current photo will be centered when the
			// lightbox closes. Applied via afterNavigate (history.go(-1) case) or directly
			// in the close handler (replaceState case) — both fire after SvelteKit's own
			// scroll restoration, ensuring we override it.
			pendingFocusIndex = pswp.currIndex;
			if (container) {
				const box = layout().boxes[pswp.currIndex];
				const galleryTop = container.getBoundingClientRect().top + window.scrollY;
				const photoCenterY = galleryTop + box.top + box.height / 2;
				pendingScrollY = photoCenterY - window.innerHeight / 2;
			}
		});

		// Inject a copy-link button into PhotoSwipe's top bar (left of the close button).
		// Copies window.location.href, which is kept current by the replaceState calls above.
		const topBar = pswp.element?.querySelector('.pswp__top-bar');
		if (topBar) {
			const linkSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`;
			const checkSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><polyline points="20 6 9 17 4 12"/></svg>`;

			const copyBtn = document.createElement('button');
			copyBtn.className = 'pswp__button pswp-copy-link';
			copyBtn.title = 'Copy link';
			copyBtn.innerHTML = linkSVG;

			copyBtn.onclick = () => {
				navigator.clipboard
					.writeText(window.location.href)
					.then(() => {
						copyBtn.innerHTML = checkSVG;
						copyBtn.classList.add('copied');
						setTimeout(() => {
							copyBtn.innerHTML = linkSVG;
							copyBtn.classList.remove('copied');
						}, 1500);
					})
					.catch(() => {
						// Clipboard not available (old browser or denied permission) — silently ignore
					});
			};

			// Insert just before the close button (last child of top bar)
			topBar.insertBefore(copyBtn, topBar.lastElementChild);
		}

		// Inject a caption into each of PhotoSwipe's 3-slide holder elements (prev,
		// current, next) so captions swipe with their photo rather than staying fixed.
		// Uses pswp.mainScroll.itemHolders (PhotoSwipe v5 internal API).
		const holders = (pswp as any).mainScroll?.itemHolders as any[] | undefined;
		if (holders) {
			// Inject one caption element into each holder's DOM element up front.
			holders.forEach((holder: any) => {
				const el = document.createElement('div');
				el.className = 'pswp-caption';
				el.style.display = 'none';
				holder.el?.appendChild(el);
			});

			const updateAll = () => {
				holders.forEach((holder: any) => {
					// Query the caption from holder.el directly rather than using a
					// parallel captionEls[] array. PhotoSwipe rotates the itemHolders
					// array as you navigate, so array index no longer matches the DOM
					// element after the first swipe — querying by element avoids that.
					const el = holder.el?.querySelector('.pswp-caption') as HTMLElement | null;
					if (!el) return;
					const idx = holder.slide?.index;
					const item =
						typeof idx === 'number' && idx >= 0 && idx < photoswipeItems.length
							? photoswipeItems[idx]
							: null;
					if (!item?.caption) {
						el.style.display = 'none';
						return;
					}
					el.textContent = item.caption;
					el.style.display = '';
					const scale = Math.min(window.innerWidth / item.w, window.innerHeight / item.h);
					el.style.bottom = `${(window.innerHeight - item.h * scale) / 2}px`;
				});
			};

			// Fade captions in/out when zooming. Captions live outside the zoom
			// transform so they stay fixed while the image moves, which looks wrong.
			// On zoom-out, delay the fade-in so it waits for the animation to settle.
			let captionFadeTimer: ReturnType<typeof setTimeout> | null = null;
			const setCaptionOpacity = (opacity: string) => {
				holders.forEach((holder: any) => {
					const el = holder.el?.querySelector('.pswp-caption') as HTMLElement | null;
					if (el && el.style.display !== 'none') el.style.opacity = opacity;
				});
			};
			// beforeZoomTo fires at the start of any zoom animation — fade out immediately
			pswp.on('beforeZoomTo' as any, () => {
				if (captionFadeTimer) { clearTimeout(captionFadeTimer); captionFadeTimer = null; }
				setCaptionOpacity('0');
			});
			// zoomPanUpdate fires during pinch and after tap-zoom settles
			pswp.on('zoomPanUpdate', () => {
				const slide = (pswp as any).currSlide;
				const isZoomed = slide?.currZoomLevel > slide?.zoomLevels?.initial * 1.01;
				if (isZoomed) {
					// Covers pinch-to-zoom (beforeZoomTo doesn't fire for pinch)
					if (captionFadeTimer) { clearTimeout(captionFadeTimer); captionFadeTimer = null; }
					setCaptionOpacity('0');
				} else if (!captionFadeTimer) {
					captionFadeTimer = setTimeout(() => {
						captionFadeTimer = null;
						setCaptionOpacity('1');
					}, 0);
				}
			});
			// Reset caption opacity when navigating between slides
			pswp.on('change', () => {
				if (captionFadeTimer) { clearTimeout(captionFadeTimer); captionFadeTimer = null; }
				setCaptionOpacity('1');
				requestAnimationFrame(updateAll);
			});
			pswp.on('resize', updateAll);
			pswp.on('openingAnimationEnd', updateAll);
			// Show caption for the initial photo via rAF. This covers two cases:
			// 1. animate=false (showAnimationDuration=0): openingAnimationEnd fires
			//    synchronously inside pswp.init(), before this listener is registered,
			//    so it never fires — rAF is the only trigger.
			// 2. animate=true: openingAnimationEnd fires after the animation but
			//    holder.slide may not yet be assigned; rAF defers past that window
			//    (same reason change uses rAF).
			requestAnimationFrame(updateAll);
		}
	}

	// Re-initialize image state whenever the album changes (covers both initial mount
	// and client-side navigation between albums, where onMount doesn't re-run).
	// Clears stale src/loaded arrays first so old album photos never bleed through,
	// and cancels any pending slow-mode timeouts from the previous album.
	$effect(() => {
		const photos = album?.photos ?? [];

		// Reset imageLoaded when navigating to a different album, or when album just
		// became available after decryption (photo count was 0 while encrypted).
		// Uses non-reactive lastEffectPhotosLen (like lastEffectSlug) to avoid creating
		// a reactive dependency on imageLoaded, which would cause an infinite effect loop.
		const slugChanged = data.slug !== lastEffectSlug;
		const albumJustLoaded = album !== null && photos.length !== lastEffectPhotosLen;
		if (slugChanged || albumJustLoaded) {
			lastEffectSlug = data.slug;
			lastEffectPhotosLen = photos.length;
			imageLoaded = photos.map(() => false);
		}

		if (slowMode) {
			// Start all srcs empty; fill each one after a random delay.
			// The setTimeout callbacks run outside the effect's tracking context
			// so writing imageSrcs[i] there does not re-trigger this effect.
			// Delay setting src so the browser doesn't start fetching until
			// after the timeout — this triggers a real load cycle, not just
			// a visual delay. loading="lazy" is also disabled in slow mode
			// to avoid unpredictable interaction with programmatic src assignment.
			imageSrcs = photos.map(() => '');
			const timeouts = photos.map((photo: any, i: number) => {
				const src = `/albums/${data.slug}/${photo.src.grid}`;
				const delay = 500 + Math.random() * 2000;
				return setTimeout(() => {
					imageSrcs[i] = src;
				}, delay);
			});
			return () => {
				timeouts.forEach(clearTimeout);
			};
		} else {
			// Build the full array in one assignment — avoids reading imageSrcs
			// inside the effect (which would create a dependency and cause an
			// infinite update loop when the assignment then triggers a re-run).
			imageSrcs = photos.map(
				(photo: any) => `/albums/${data.slug}/${photo.src.grid}`
			);
		}
	});

	onMount(() => {
		syncSiteId(data.siteId);

		const updateWidth = () => {
			if (container) {
				containerWidth = container.clientWidth;
			}
		};
		const handleKeydown = (e: KeyboardEvent) => {
			// Ignore ESC if lightbox is open or was just closed (same ESC keypress)
			if (e.key === 'Escape' && !lightboxOpen && Date.now() - lightboxClosedAt > 300) {
				goto('/');
			}
		};
		updateWidth();
		layoutReady = true;

		if (data.encryptedBlob) {
			// Silently try stored passwords (fire and forget — resolves async).
			// On success, tryDecryptAlbum also handles auto-opening a permalink.
			tryDecryptAlbum();
		} else {
			// Open lightbox at the photo specified in the route (e.g. /albums/antarctica/15).
			// Skip the opening animation so it appears instantly rather than fading/zooming in.
			// invalidPhotoIndex (derived) handles the out-of-range case in the template.
			if (data.photoIndex !== null && invalidPhotoIndex === null) {
				openLightbox(data.photoIndex, false);
			}
		}

		window.addEventListener('resize', updateWidth);
		window.addEventListener('keydown', handleKeydown);
		return () => {
			window.removeEventListener('resize', updateWidth);
			window.removeEventListener('keydown', handleKeydown);
			// If the user navigates away via a link while the lightbox is open, the close
			// event never fires (navigation unmounts the component first).  Clean up the
			// popstate listener and destroy PhotoSwipe directly so it doesn't float over
			// the new page.  PhotoSwipe lives in document.body, outside Svelte's tree.
			if (activePopstateHandler) {
				window.removeEventListener('popstate', activePopstateHandler);
				activePopstateHandler = null;
			}
			pswpInstance?.destroy();
			pswpInstance = null;
		};
	});
</script>

<OpenGraph
	title={albumTitle}
	description={description ||
		(album
			? `${album.photos.length} photos from the '${albumTitle}' album`
			: albumTitle)}
	url="{data.siteConfig.siteUrl}/albums/{data.slug}"
	siteName={data.siteConfig.siteName}
	image={album ? `${data.siteConfig.siteUrl}/albums/${data.slug}/cover.jpg` : undefined}
/>

{#if album}
	<main>
		<header>
			<a href="/">← Albums</a>
			<h1>{albumTitle}</h1>
			{#if description}
				<p class="description">{@html description}</p>
			{/if}
			<p class="meta">
				{album.photos.length} photos{dateSpan ? ` · ${dateSpan}` : ''}
			</p>
		</header>

		{#if invalidPhotoIndex !== null}
			<div class="not-found">
				<p>No photo #{invalidPhotoIndex} in '{album.title}'.</p>
				<a href="/albums/{data.slug}">Back to the album</a>
			</div>
		{/if}

		<div
			class="gallery"
			bind:this={container}
			style="height: {layout().containerHeight}px;"
			class:layout-ready={layoutReady}
		>
			{#each album.photos as photo, i}
				{@const box = layout().boxes[i]}
				<button
					class="photo"
					style="
						position: absolute;
						left: {box.left}px;
						top: {box.top}px;
						width: {box.width}px;
						height: {box.height}px;
					"
					onclick={() => openLightbox(i)}
				>
					<!-- src starts empty; set in onMount (immediately or after delay in ?slow mode).
					     loading="lazy" is dropped in slow mode to avoid browser deferring
					     images that are in-viewport when the delayed src is finally assigned.
					     The `loaded` class drives the fade-in transition in CSS. -->
					<img
						src={imageSrcs[i]}
						alt={photo.description || photo.fileName}
						width={box.width}
						height={box.height}
						loading={slowMode ? undefined : 'lazy'}
						class:loaded={imageLoaded[i]}
						onload={() => {
							imageLoaded[i] = true;
						}}
					/>
					{#if photo.description}
						<div class="photo-caption">{photo.description}</div>
					{/if}
				</button>
			{/each}
		</div>

		<BackToTop />
	</main>
{:else}
	<main class="loading-page">
		{#if !data.encryptedBlob}
			<header>
				<a href="/">← Albums</a>
				<h1>{albumTitle}</h1>
			</header>
		{/if}
	</main>
{/if}

{#if browser && data.encryptedBlob && !album && !unlocking}
	<div class="fullscreen-overlay">
		<PasswordPrompt
			prefix="Album"
			name={albumTitle}
			hint={data.albumHint}
			{shakeCount}
			onunlock={handleUnlock}
		/>
	</div>
{/if}

<style>
	main {
		max-width: 2000px;
		margin: 0 auto;
		padding: 1rem;
	}

	.loading-page {
		min-height: 80vh;
	}

	/* Full-screen overlay covering header/footer when album is locked */
	.fullscreen-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-color);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	header {
		margin-bottom: 1rem;
	}

	header a {
		color: var(--text-muted);
		text-decoration: none;
		font-size: 0.9rem;
	}

	header a:hover {
		color: var(--text-color);
	}

	header h1 {
		margin: 0.5rem 0 0.25rem 0;
	}

	header p {
		margin: 0;
		color: var(--text-muted);
	}

	header .description {
		margin-top: 0.3rem;
		font-size: 0.95rem;
		color: var(--text-color-2nd);
		opacity: 0.8;
	}

	header .meta {
		margin-top: 0.4rem;
		text-align: right;
		font-style: italic;
		font-size: 0.85rem;
	}

	.not-found {
		padding: 3rem 1rem;
		text-align: center;
		color: var(--text-muted);
	}

	.not-found p {
		margin: 0 0 1rem 0;
		font-size: 1.1rem;
	}

	.not-found a {
		color: var(--link-color);
		text-decoration: none;
	}

	.not-found a:hover {
		text-decoration: underline;
	}

	.gallery {
		position: relative;
		width: 100%;
	}

	/* Placeholder background on the container, not the img. Since the img starts at
	   opacity: 0 (fully transparent), a background on the img itself is invisible.
	   The container color shows through until the image fades in on top of it.
	   Gated on .layout-ready to avoid showing placeholder boxes during the initial
	   containerWidth recalculation (which would cause visible size shifting). */
	.photo {
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
		display: block;
		overflow: hidden;
	}

	.layout-ready .photo {
		background: var(--img-placeholder);
	}

	/* Images start invisible and fade in once loaded.
	   The `loaded` class is added via onload, triggering the transition. */
	.photo img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0;
		transition: opacity 0.4s ease;
	}

	.photo img.loaded {
		opacity: 1;
	}

	.photo:hover img.loaded {
		opacity: 1;
	}

	/* Hover caption overlay — slides up from bottom on hover */
	.photo-caption {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1.5rem 0.6rem 0.5rem;
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.75));
		color: white;
		font-size: 0.78rem;
		line-height: 1.3;
		text-align: left;
		opacity: 0;
		transform: translateY(4px);
		transition: opacity 0.25s ease, transform 0.25s ease;
		pointer-events: none;
	}

	.photo:hover .photo-caption {
		opacity: 1;
		transform: translateY(0);
	}

	/* On touch devices (no hover), always show captions in the grid */
	@media (hover: none) {
		.photo-caption {
			opacity: 1;
			transform: translateY(0);
		}
	}

	/* PhotoSwipe customizations for dark theme */
	:global(.pswp) {
		--pswp-bg: #000;
	}

	/* Fully opaque background - hide content underneath */
	:global(.pswp__bg) {
		opacity: 1 !important;
	}

	/* Make nav arrows less prominent and nudge inward */
	:global(.pswp__button--arrow) {
		opacity: 0.3 !important;
	}

	:global(.pswp__button--arrow--prev) {
		left: 7px !important;
	}

	:global(.pswp__button--arrow--next) {
		right: 7px !important;
	}

	/* Copy-link button in the PhotoSwipe top bar */
	:global(.pswp-copy-link) {
		opacity: 0.6;
		transition: opacity 0.2s, color 0.2s;
		color: white;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	:global(.pswp-copy-link:hover) {
		opacity: 1;
	}

	:global(.pswp-copy-link.copied) {
		opacity: 1;
		color: #6ddb6d;
	}

	/* Lightbox caption — bottom set dynamically in JS to align with photo bottom edge */
	:global(.pswp-caption) {
		position: absolute;
		left: 0;
		right: 0;
		padding: 1.5rem 3rem 0.75rem;
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.75));
		color: white;
		font-size: 0.9rem;
		line-height: 1.4;
		text-align: center;
		pointer-events: none;
		z-index: 10;
		transition: opacity 0.3s ease;
	}
</style>
