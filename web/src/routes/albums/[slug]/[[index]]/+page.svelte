<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { goto, replaceState, pushState } from '$app/navigation';
	import justifiedLayout from 'justified-layout';
	import PhotoSwipe from 'photoswipe';
	import 'photoswipe/style.css';

	let { data } = $props();
	let containerWidth = $state(1200);
	let container: HTMLDivElement;
	let showBackToTop = $state(false);
	let lightboxOpen = $state(false);
	let lightboxClosedAt = $state(0);
	let pswpInstance: PhotoSwipe | null = null; // reference to the open PhotoSwipe instance
	// Stored so onMount cleanup can remove it when navigating away via a link while the
	// lightbox is open (component unmounts before the close event fires).
	let activePopstateHandler: (() => void) | null = null;
	// Image fade-in state. Populated by the $effect below, which re-runs on album change.
	let imageSrcs = $state<string[]>([]);   // src per image; empty string = not yet assigned
	let imageLoaded = $state<boolean[]>([]); // true once the browser fires the load event
	let slowMode = $state(browser && new URLSearchParams(window.location.search).has('slow')); // true when ?slow is in the URL
	let layoutReady = $state(false);         // true after onMount measures the real container width
	let lastEffectSlug = '';                 // non-reactive: tracks which slug $effect last reset imageLoaded for
	// 1-based photo number for display when the route index is out of range; null otherwise.
	// Derived (not state) so it updates reactively when data changes on client-side navigation.
	let invalidPhotoIndex = $derived(
		data.photoIndex !== null && (data.photoIndex < 0 || data.photoIndex >= data.album.photos.length)
			? data.photoIndex + 1
			: null
	);

	// Compute layout based on photo aspect ratios
	let layout = $derived(() => {
		const aspectRatios = data.album.photos.map((p: any) => p.width / p.height);
		return justifiedLayout(aspectRatios, {
			containerWidth,
			targetRowHeight: 300,
			containerPadding: 0,
			boxSpacing: 8
		});
	});

	// Build PhotoSwipe data source
	let photoswipeItems = $derived(
		data.album.photos.map((photo: any) => ({
			src: `/albums/${data.slug}/${photo.src.full}`,
			w: photo.width,
			h: photo.height,
			msrc: `/albums/${data.slug}/${photo.src.grid}`, // thumbnail for loading
			alt: photo.description || photo.fileName,
			caption: photo.description || ''
		}))
	);

	function openLightbox(index: number, animate = true) {
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
			pswp.close();        // plays close animation; close handler cleans up the rest
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

		pswp.on('openingAnimationStart', () => { lightboxOpen = true; });
		pswp.on('close', () => {
			window.removeEventListener('popstate', handlePopstate);
			activePopstateHandler = null;
			pswpInstance = null;
			lightboxOpen = false;
			lightboxClosedAt = Date.now();
			if (!closedByBackNav) {
				if (pushedHistoryEntry) {
					// Pop our native push entry so the album URL is restored and history
					// is clean (subsequent back goes to the page before the album, not
					// back to the photo URL).  Fires a popstate that SvelteKit handles
					// as a normal navigation to /albums/slug.
					history.go(-1);
				} else {
					// Permalink open (animate=false, no native push): update URL from
					// /albums/slug/N back to /albums/slug via SvelteKit's shallow replace.
					replaceState(`/albums/${data.slug}`, {});
				}
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
		// NOTE: SvelteKit's popstate handler ALSO fires for this entry (it recognises its
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
				navigator.clipboard.writeText(window.location.href).then(() => {
					copyBtn.innerHTML = checkSVG;
					copyBtn.classList.add('copied');
					setTimeout(() => {
						copyBtn.innerHTML = linkSVG;
						copyBtn.classList.remove('copied');
					}, 1500);
				}).catch(() => {
					// Clipboard not available (old browser or denied permission) — silently ignore
				});
			};

			// Insert just before the close button (last child of top bar)
			topBar.insertBefore(copyBtn, topBar.lastElementChild);
		}

		// Inject a caption into each of PhotoSwipe's 3 slide holder elements (prev,
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

			pswp.on('change', () => requestAnimationFrame(updateAll));
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

	function scrollToTop() {
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	// Re-initialize image state whenever the album changes (covers both initial mount
	// and client-side navigation between albums, where onMount doesn't re-run).
	// Clears stale src/loaded arrays first so old album photos never bleed through,
	// and cancels any pending slow-mode timeouts from the previous album.
	$effect(() => {
		// Only reset imageLoaded when navigating to a different album.  If the same
		// album's data is re-fetched (e.g. SvelteKit fallback navigation after reload +
		// back), the slug is unchanged — skipping the reset preserves the loaded state
		// so images don't get stuck at opacity:0 (Svelte diffs src attrs as equal,
		// onload never re-fires, but imageLoaded[i]=true still applies the loaded class).
		if (data.slug !== lastEffectSlug) {
			lastEffectSlug = data.slug;
			imageLoaded = data.album.photos.map(() => false);
		}

		if (slowMode) {
			// Start all srcs empty; fill each one after a random delay.
			// The setTimeout callbacks run outside the effect's tracking context
			// so writing imageSrcs[i] there does not re-trigger this effect.
			// Delay setting src so the browser doesn't start fetching until
			// after the timeout — this triggers a real load cycle, not just
			// a visual delay. loading="lazy" is also disabled in slow mode
			// to avoid unpredictable interaction with programmatic src assignment.
			imageSrcs = data.album.photos.map(() => '');
			const timeouts = data.album.photos.map((photo: any, i: number) => {
				const src = `/albums/${data.slug}/${photo.src.grid}`;
				const delay = 500 + Math.random() * 2000;
				return setTimeout(() => { imageSrcs[i] = src; }, delay);
			});
			return () => { timeouts.forEach(clearTimeout); };
		} else {
			// Build the full array in one assignment — avoids reading imageSrcs
			// inside the effect (which would create a dependency and cause an
			// infinite update loop when the assignment then triggers a re-run).
			imageSrcs = data.album.photos.map((photo: any) =>
				`/albums/${data.slug}/${photo.src.grid}`
			);
		}
	});

	onMount(() => {
		const updateWidth = () => {
			if (container) {
				containerWidth = container.clientWidth;
			}
		};
		const updateScroll = () => {
			showBackToTop = window.scrollY > 600;
		};
		const handleKeydown = (e: KeyboardEvent) => {
			// Ignore ESC if lightbox is open or was just closed (same ESC keypress)
			if (e.key === 'Escape' && !lightboxOpen && Date.now() - lightboxClosedAt > 300) {
				goto('/');
			}
		};
		updateWidth();
		layoutReady = true;

		// Open lightbox at the photo specified in the route (e.g. /albums/antarctica/15).
		// Skip the opening animation so it appears instantly rather than fading/zooming in.
		// invalidPhotoIndex (derived) handles the out-of-range case in the template.
		if (data.photoIndex !== null && invalidPhotoIndex === null) {
			openLightbox(data.photoIndex, false);
		}

		window.addEventListener('resize', updateWidth);
		window.addEventListener('scroll', updateScroll, { passive: true });
		window.addEventListener('keydown', handleKeydown);
		return () => {
			window.removeEventListener('resize', updateWidth);
			window.removeEventListener('scroll', updateScroll);
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

<svelte:head>
	<title>{data.album.title}</title>
	<meta property="og:title" content={data.album.title} />
	<meta property="og:description" content={data.description || `${data.album.photos.length} photos from the '${data.album.title}' album`} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content="{import.meta.env.VITE_SITE_URL}/albums/{data.slug}" />
	<meta property="og:image" content="{import.meta.env.VITE_SITE_URL}/albums/{data.slug}/cover.jpg" />
	<meta property="og:site_name" content={import.meta.env.VITE_SITE_NAME} />
	<meta name="twitter:card" content="summary_large_image" />
	<link rel="canonical" href="{import.meta.env.VITE_SITE_URL}/albums/{data.slug}" />
</svelte:head>

<main>
	<header>
		<a href="/">← Albums</a>
		<h1>{data.album.title}</h1>
		{#if data.description}
			<p class="description">{data.description}</p>
		{/if}
		<p class="meta">{data.album.photos.length} photos{data.dateSpan ? ` · ${data.dateSpan}` : ''}</p>
	</header>

	{#if invalidPhotoIndex !== null}
		<div class="not-found">
			<p>No photo #{invalidPhotoIndex} in '{data.album.title}'.</p>
			<a href="/albums/{data.slug}">Back to the album</a>
		</div>
	{/if}

	<div class="gallery" bind:this={container} style="height: {layout().containerHeight}px;" class:layout-ready={layoutReady}>
		{#each data.album.photos as photo, i}
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
					onload={() => { imageLoaded[i] = true; }}
				/>
				{#if photo.description}
					<div class="photo-caption">{photo.description}</div>
				{/if}
			</button>
		{/each}
	</div>

	{#if showBackToTop}
		<button class="back-to-top" onclick={scrollToTop} aria-label="Back to top">
			↑
		</button>
	{/if}
</main>

<style>
	main {
		max-width: 2000px;
		margin: 0 auto;
		padding: 1rem;
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
		opacity: 0.85;
	}

	/* Hover caption overlay — slides up from bottom on hover */
	.photo-caption {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1.5rem 0.6rem 0.5rem;
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.65));
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

	.back-to-top {
		position: fixed;
		bottom: 1.2rem;
		right: calc(1.5rem);
		z-index: 50;
		background: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: 50%;
		width: 44px;
		height: 44px;
		font-size: 1.4rem;
		color: var(--text-color);
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		padding-bottom: 2px;
		box-shadow: 0 2px 8px var(--shadow-color);
		opacity: 0.7;
		transition: opacity 0.2s, transform 0.2s;
	}

	@media (min-width: 769px) {
		.back-to-top {
			padding-bottom: 5px;
		}
	}

	.back-to-top:hover {
		opacity: 1;
		transform: scale(1.1);
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
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.6));
		color: white;
		font-size: 0.9rem;
		line-height: 1.4;
		text-align: center;
		pointer-events: none;
		z-index: 10;
	}
</style>
