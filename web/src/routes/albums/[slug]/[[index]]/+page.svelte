<!-- Suppress IntelliJ's "Selector is never used" warning for :global(.pswp) and similar
     selectors that target DOM elements injected dynamically by third-party libraries
     (PhotoSwipe). IntelliJ's static CSS analyzer can't see runtime-injected classes. -->
<!--suppress CssUnusedSymbol -->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { browser } from '$app/environment';
	import { goto, replaceState, pushState } from '$app/navigation';
	import { base, resolve } from '$app/paths';
	import justifiedLayout from 'justified-layout';
	import PhotoSwipe from 'photoswipe';
	import 'photoswipe/style.css';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';
	import BackToTop from '$lib/components/BackToTop.svelte';
	import OpenGraph from '$lib/components/OpenGraph.svelte';
	import PasswordPrompt from '$lib/components/PasswordPrompt.svelte';
	import type { AlbumIndex, Photo } from '$lib/types';
	import type { ItemHolder } from 'photoswipe';
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
	import { stripTags } from '$lib/html';
	import { navigateCursor, type Direction } from '$lib/navigation';

	let { data } = $props();

	// Unpack albumData into flat reactive locals so the rest of this component reads cleanly.
	const siteId = $derived(data.albumData.siteId);
	const slug = $derived(data.albumData.slug);
	const photoIndex = $derived(data.albumData.photoIndex);
	const albumEncrypted = $derived(data.albumData.album.encrypted);
	const encryptedAlbumBlob = $derived(
		data.albumData.album.encrypted ? data.albumData.album.blob : null
	);
	const loadedAlbum = $derived(data.albumData.album.encrypted ? null : data.albumData.album.data);
	const albumHint = $derived(
		data.albumData.album.encrypted ? data.albumData.album.hint : undefined
	);

	// Client-decrypted album (null until the user's stored password or manual entry works).
	let decryptedAlbum = $state<AlbumIndex | null>(null);
	// Effective album: server-provided (unencrypted) takes precedence, else client-decrypted.
	let album = $derived(loadedAlbum ?? decryptedAlbum);
	// Metadata: prefer server-loaded values; fall back to fields embedded in the decrypted index
	// (needed for site-encrypted sites where albums.enc.json is not fetched server-side).
	let albumTitle = $derived(album?.title ?? data.albumData.albumTitle);
	let description = $derived(data.albumData.description || album?.description || '');
	let plainDescription = $derived(stripTags(description));
	let dateSpan = $derived(data.albumData.dateSpan || album?.dateSpan || '');
	// Header navigation configured via customizations.album_nav; empty means the default
	// "← Albums" link. Rides in config.json, so it is available before any decryption.
	let albumNav = $derived(data.siteConfig?.albumNav ?? []);
	// True while we're silently trying stored passwords so we don't flash the prompt.
	// $effect.pre runs synchronously before Svelte's first DOM commit in the browser,
	// so if a stored password exists we set unlocking=true before the prompt ever renders.
	// (On SSR, effects don't run; unlocking stays false and the prompt renders in the
	// static HTML — this is fine since JS will correct it immediately on hydration.)
	let unlocking = $state(false);
	$effect.pre(() => {
		if (
			albumEncrypted &&
			(getStoredPassword(albumKey(siteId, slug)) || getStoredPassword(siteKey(siteId)))
		) {
			unlocking = true;
		}
	});
	// Hide footer until album is ready on encrypted pages, preventing a layout jump.
	$effect.pre(() => {
		if (albumEncrypted) {
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
	let lastEffectSlug = ''; // non-reactive: tracks which slug $effect last reset imageLoaded for
	let lastEffectPhotosLen = 0; // non-reactive: tracks last known photo count to detect album load
	// 1-based photo number for display when the route index is out of range; null otherwise.
	let invalidPhotoIndex = $derived(
		album !== null && photoIndex !== null && (photoIndex < 0 || photoIndex >= album.photos.length)
			? photoIndex + 1
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

	// Height reserved for the browser's native video control bar, applied as bottom padding
	// on a video slide's caption so the text clears the play button and scrubber. The
	// caption box itself still reaches the bottom of the video, which is what lets its
	// gradient run continuously into the shade the browser draws behind those controls.
	// Chrome's bar is ~40px; the extra few pixels keep a gap between text and controls.
	const VIDEO_CONTROLS_HEIGHT = 48;

	// Formats a duration in seconds as m:ss, e.g. 73 -> "1:13".
	// Truncates rather than rounds so the badge matches what the browser's own control bar
	// shows for the same clip: a 17.6s video reads 0:17 in both places, not 0:18 here and
	// 0:17 there.
	function formatDuration(seconds: number): string {
		const total = Math.floor(seconds);
		return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
	}

	// Accessible label for a grid tile. Videos say so, since the play badge is aria-hidden
	// and a screen reader would otherwise have no way to tell a clip from a still.
	function photoLabel(photo: Photo): string {
		const base = stripTags(photo.description) || photo.fileName;
		if (photo.kind !== 'video') return base;
		const length = photo.duration ? `, ${formatDuration(photo.duration)}` : '';
		return `Video: ${base}${length}`;
	}

	// Build PhotoSwipe data source.
	// Video slides carry videoSrc and are rendered by the contentLoad hook in openLightbox;
	// their w/h are the poster's, which is what PhotoSwipe uses to size the slide.
	//
	// `type: 'video'` is load-bearing, not decoration. PhotoSwipe infers type from the data
	// (Content constructor: data.type, else 'image' when data.src is set), and every item
	// here has a src for the poster. Left to infer, a video slide is classed as image
	// content, and setDisplayedSize later calls loadImage() on it *after* our contentLoad
	// handler has already swapped in a <div> wrapper. loadImage assigns .src/.onload to
	// that div and sets state back to LOADING; a div has no complete/onload, so the state
	// never resolves. Visible result: the loading spinner appears after 2s and spins
	// forever, and the placeholder is never dropped. An explicit type makes
	// isImageContent() false, so none of that path runs.
	//
	// Second, intended consequence: Content.isZoomable() is `isImageContent() && not
	// errored`, so this also disables zoom on video slides — the zoom button, double-tap,
	// pinch and the `z` key all become inert. That is what we want. Zooming scales the
	// element, which would enlarge the browser's native control bar along with the
	// picture and push play/scrub off screen. Photos are unaffected.
	let photoswipeItems = $derived(
		(album?.photos ?? []).map((photo) => ({
			type: photo.kind === 'video' ? 'video' : undefined,
			src: `/albums/${slug}/${photo.src.full}`,
			w: photo.width,
			h: photo.height,
			msrc: `/albums/${slug}/${photo.src.grid}`, // thumbnail for loading
			alt: stripTags(photo.description) || photo.fileName,
			caption: photo.description || '',
			videoSrc: photo.src.video ? `/albums/${slug}/${photo.src.video}` : undefined,
			posterSrc: `/albums/${slug}/${photo.src.full}`
		}))
	);

	function cacheAlbumCover(album: AlbumIndex) {
		const cover = album.cover ?? album.photos[0]?.src.grid;
		if (cover) storeAlbumCover(siteId, slug, `/albums/${slug}/${cover}`);
	}

	async function tryDecryptAlbum() {
		if (!albumEncrypted) return;
		unlocking = true;

		// Try per-album stored password first, then site-wide.
		for (const key of [albumKey(siteId, slug), siteKey(siteId)]) {
			const password = getStoredPassword(key);
			if (password) {
				const result = await tryDecrypt(encryptedAlbumBlob!, password);
				if (result) {
					decryptedAlbum = result as AlbumIndex;
					storePassword(albumKey(siteId, slug), password);
					cacheAlbumCover(decryptedAlbum);
					unlocking = false;
					// Wait for Svelte to recompute photoswipeItems from the decrypted album,
					// then auto-open the lightbox if this is a permalink URL.
					await tick();
					if (photoIndex !== null && invalidPhotoIndex === null) {
						openLightbox(photoIndex, false);
					}
					return;
				}
			}
		}

		unlocking = false;
	}

	async function handleUnlock(password: string) {
		if (!albumEncrypted) return;
		const result = await tryDecrypt(encryptedAlbumBlob!, password);
		if (result) {
			decryptedAlbum = result as AlbumIndex;
			storePassword(albumKey(siteId, slug), password);
			cacheAlbumCover(decryptedAlbum);
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

		// --- Video slides ---------------------------------------------------------------
		// Recomputes every caption's opacity. Forward-declared as a no-op because the caption
		// machinery is set up further down (it needs pswp.mainScroll.itemHolders, which only
		// exists once the lightbox is built) while the <video> elements that drive it are
		// created here. Assigned to applyCaptionOpacity in that block below.
		let refreshCaptions: () => void = () => {};

		// PhotoSwipe has no built-in video type, so we take over content rendering for
		// items that carry a videoSrc and hand back our own <video> element.
		pswp.on('contentLoad', (e) => {
			const data = e.content.data as (typeof photoswipeItems)[number];
			if (!data.videoSrc) return;
			e.preventDefault(); // stop PhotoSwipe loading the poster as a normal image

			const video = document.createElement('video');
			video.className = 'pswp-video';
			video.src = data.videoSrc;
			video.poster = data.posterSrc;
			video.controls = true;
			// Muted by default so a swipe through an album never blares audio unexpectedly;
			// the native controls let the viewer unmute. playsinline keeps iOS Safari from
			// hijacking the whole screen with its own fullscreen player, which would leave
			// PhotoSwipe's state out of sync on exit.
			video.muted = true;
			video.playsInline = true;
			video.preload = 'metadata';

			// Clicks on the video must not reach PhotoSwipe's bgClickAction, or tapping
			// play would close the lightbox instead.
			video.addEventListener('click', (ev) => ev.stopPropagation());

			// Hide the caption while the clip plays. The caption sits above the browser's
			// native control bar (see controlsInset below), which reads correctly while that
			// bar is up but leaves the caption floating mid-frame over nothing once the
			// browser auto-hides the controls a second or two into playback. There is no API
			// for native-control visibility, so this tracks what drives it instead: every
			// browser keeps the bar up whenever a video is paused or ended, and only hides it
			// during playback, so paused/ended is the same set of moments.
			//
			// 'ended' is listened for separately because it does not reliably raise 'pause',
			// yet it is a state where the controls come back.
			//
			// Deliberately not restored when the viewer jiggles the mouse mid-playback and the
			// controls reappear: matching that would mean reproducing each browser's own
			// inactivity heuristics, and a caption that stays hidden never looks broken.
			//
			// No teardown needed — the listeners die with the element, like the click one above.
			for (const event of ['play', 'pause', 'ended']) {
				video.addEventListener(event, () => refreshCaptions());
			}

			// Wrapped in a div because PhotoSwipe types content.element as an image or a
			// div and sizes it from the slide's zoom level. Letting it size the wrapper and
			// having the video fill that keeps us on PhotoSwipe's normal layout path
			// instead of casting our way around it.
			const wrap = document.createElement('div');
			wrap.className = 'pswp-video-wrap';
			wrap.appendChild(video);
			e.content.element = wrap;

			// Required after preventDefault. PhotoSwipe leaves the content in its LOADING
			// state otherwise, which has two visible consequences: the loading spinner
			// appears after preloaderDelay (2s) and never clears, and isKeepingPlaceholder
			// stays true so the low-res placeholder <img> is never dropped, sitting on top
			// of the video and swallowing clicks meant for the native controls.
			// The poster attribute means there is already something to look at, so marking
			// it loaded now rather than waiting on loadedmetadata costs nothing.
			e.content.onLoaded();
		});

		// Pause when a slide scrolls out of view. Without this, swiping to the next photo
		// leaves the previous clip playing (and audible, once unmuted) off-screen.
		const pauseVideoIn = (element: HTMLElement | null | undefined) => {
			element?.querySelector('video')?.pause();
		};
		pswp.on('contentDeactivate', (e) => pauseVideoIn(e.content.element));
		pswp.on('contentDestroy', (e) => pauseVideoIn(e.content.element));

		// Space toggles play/pause on a video slide, the convention every video player uses.
		//
		// Hooked to PhotoSwipe's own keydown event rather than a document listener: it is
		// scoped to this instance's lifetime, so it needs no teardown, and preventing it
		// also stops PhotoSwipe acting on the key. Space is not bound by PhotoSwipe today,
		// but this keeps us correct if that changes.
		//
		// Both preventDefaults do different jobs: the PhotoSwipe one stops its handling, the
		// original-event one stops the browser scrolling the page behind the lightbox.
		pswp.on('keydown', (e) => {
			const original = e.originalEvent;
			if (original.key !== ' ' && original.key !== 'Spacebar') return;

			const video = pswp.currSlide?.content?.element?.querySelector('video');
			if (!video) return; // photo slide: leave the key alone

			// When the video already has focus the browser toggles it itself, and it does so
			// on keyup. Handling it here as well would play on keydown and pause on keyup,
			// cancelling out and making the key look dead. Defer to the native behaviour
			// rather than trying to suppress it.
			if (document.activeElement === video) return;

			e.preventDefault();
			original.preventDefault();
			if (video.paused) {
				void video.play().catch(() => {
					// Autoplay policy can still refuse; the visible controls remain the
					// fallback, so a rejection here is not worth surfacing.
				});
			} else {
				video.pause();
			}
		});
		pswp.on('close', () => {
			for (const holder of pswp.mainScroll?.itemHolders ?? []) {
				pauseVideoIn(holder.slide?.content?.element);
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
					replaceState(resolve(`/albums/${slug}`), {});
				}

				const focusIdx = pendingFocusIndex;
				pendingFocusIndex = null;
				// Cache the button to focus (queried once, before the guard loop starts).
				const focusBtn =
					focusIdx !== null && container
						? (container.querySelectorAll<HTMLElement>('.photo')[focusIdx] ?? null)
						: null;

				// Run a guard loop for 750ms to fight both PhotoSwipe's built-in focus
				// restoration (fires first frame) and SvelteKit's async focus reset (fires
				// ~300-500ms in, same as its scroll reset). Re-apply only when focus has
				// moved elsewhere to avoid redundant focus events.
				// Also re-applies scroll if SvelteKit resets it.
				const deadline = performance.now() + 750;
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

		// Add copy-link button to the PhotoSwipe top bar, just left of the close button (order 20).
		// Copies window.location.href, kept current by the replaceState calls in the change handler.
		// Must use the uiRegister event: pswp.ui doesn't exist until inside pswp.init().
		const linkSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`;
		const checkSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><polyline points="20 6 9 17 4 12"/></svg>`;
		pswp.on('uiRegister', () => {
			pswp.ui?.registerElement({
				name: 'copy-link',
				order: 19,
				isButton: true,
				title: 'Copy link',
				html: linkSVG,
				// _event is unused — the underscore marks that intentionally. It can't just be
				// dropped, since `el` is the second positional argument.
				onClick: (_event, el) => {
					navigator.clipboard
						.writeText(window.location.href)
						.then(() => {
							el.innerHTML = checkSVG;
							el.classList.add('copied');
							setTimeout(() => {
								el.innerHTML = linkSVG;
								el.classList.remove('copied');
							}, 1500);
						})
						.catch(() => {
							// Clipboard not available (old browser or denied permission) — silently ignore
						});
				}
			});
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
			pushState(resolve(`/albums/${slug}/${index + 1}`), {});
			pushedHistoryEntry = true;
		}
		pswp.on('change', () => {
			// SvelteKit's replaceState keeps the photo URL current as the user navigates.
			// Uses replaceState (not pushState) so every photo doesn't add a history entry
			// — back always jumps directly to the album rather than stepping photo-by-photo.
			replaceState(resolve(`/albums/${slug}/${pswp.currIndex + 1}`), {});
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

		// Inject a caption into each of PhotoSwipe's 3-slide holder elements (prev,
		// current, next) so captions swipe with their photo rather than staying fixed.
		// Uses pswp.mainScroll.itemHolders (PhotoSwipe v5 internal API).
		const holders: ItemHolder[] = pswp.mainScroll.itemHolders;
		if (holders) {
			// Inject one caption element into each holder's DOM element up front.
			holders.forEach((holder: ItemHolder) => {
				const el = document.createElement('div');
				el.className = 'pswp-caption';
				el.style.display = 'none';
				// A caption lives inside an item holder, so PhotoSwipe's pointer handling on
				// pswp.element sees events on it and treats them as a drag or a tap on the
				// slide. Stop link events there so a click actually follows the href; anything
				// else in the caption still reaches PhotoSwipe, keeping swipe and drag-to-close
				// working over the caption area.
				const stopLinkEvent = (e: Event) => {
					if ((e.target as Element).closest('a')) e.stopPropagation();
				};
				el.addEventListener('pointerdown', stopLinkEvent);
				el.addEventListener('click', stopLinkEvent);
				holder.el.appendChild(el);
			});

			// Two independent things hide a caption: zooming (below) and video playback (the
			// listeners in contentLoad). Rather than let both write style.opacity directly and
			// race — a vertical drag fires zoomPanUpdate, which would flash a hidden caption
			// back on mid-playback — one function computes the value from both inputs and is
			// the only writer. Playback state is read off the element instead of being mirrored
			// into a variable, so there is no second copy to keep in sync.
			let captionsZoomHidden = false;
			const applyCaptionOpacity = () => {
				holders.forEach((holder: ItemHolder) => {
					const el = holder.el.querySelector('.pswp-caption') as HTMLElement | null;
					if (!el || el.style.display === 'none') return;
					const video = holder.el.querySelector('video');
					const playing = !!video && !video.paused && !video.ended;
					el.style.opacity = captionsZoomHidden || playing ? '0' : '1';
				});
			};
			refreshCaptions = applyCaptionOpacity;

			const updateAll = () => {
				holders.forEach((holder: ItemHolder) => {
					// Query the caption from holder.el directly rather than using a
					// parallel captionEls[] array. PhotoSwipe rotates the itemHolders
					// array as you navigate, so array index no longer matches the DOM
					// element after the first swipe — querying by element avoids that.
					const el = holder.el.querySelector('.pswp-caption') as HTMLElement | null;
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
					el.innerHTML = item.caption;
					// Caption links always open a new tab: navigating in place tears down the
					// lightbox, losing the visitor's place in the album. Set unconditionally
					// rather than only when absent, so photogen.txt needs no boilerplate.
					el.querySelectorAll('a').forEach((a) => {
						a.target = '_blank';
						a.rel = 'noopener';
					});
					el.style.display = '';
					// A video caption's box reaches the bottom of the media exactly like a
					// photo's; what differs is that its text is held above the browser's
					// control bar by padding instead of sitting at the very bottom. Padding
					// rather than a lifted box because the gradient has to keep running down
					// behind the controls: stopping it short of them left a separate, darker
					// band above a lighter one, two stacked shades instead of one ramp.
					const isVideo = !!item.videoSrc;
					el.classList.toggle('pswp-caption--video', isVideo);
					el.style.paddingBottom = isVideo ? `${VIDEO_CONTROLS_HEIGHT + 12}px` : '';
					// Inset the caption to the displayed photo box rather than the viewport, so it
					// spans exactly the image's edges. Mirrors PhotoSwipe's own geometry: it sizes
					// the viewport from document.documentElement.clientWidth (exposed as
					// pswp.viewportSize — unlike window.innerWidth it excludes the scrollbar, which
					// otherwise leaves a gap at each edge), caps the fit zoom at 1 so images smaller
					// than the viewport aren't upscaled, and rounds the displayed size up to whole
					// pixels.
					const vp = pswp.viewportSize;
					const scale = Math.min(1, vp.x / item.w, vp.y / item.h);
					const sideInset = Math.floor((vp.x - Math.ceil(item.w * scale)) / 2);
					el.style.bottom = `${Math.floor((vp.y - Math.ceil(item.h * scale)) / 2)}px`;
					el.style.left = `${sideInset}px`;
					el.style.right = `${sideInset}px`;
					// Drop any leftover vertical-drag offset (see zoomPanUpdate below).
					el.style.transform = '';
				});
				applyCaptionOpacity();
			};

			// Fade captions in/out when zooming. Captions live outside the zoom
			// transform so they stay fixed while the image moves, which looks wrong.
			// On zoom-out, delay the fade-in so it waits for the animation to settle.
			let captionFadeTimer: ReturnType<typeof setTimeout> | null = null;
			const setCaptionsZoomHidden = (hidden: boolean) => {
				captionsZoomHidden = hidden;
				applyCaptionOpacity();
			};
			// beforeZoomTo fires at the start of any zoom animation — fade out immediately
			pswp.on('beforeZoomTo', () => {
				if (captionFadeTimer) {
					clearTimeout(captionFadeTimer);
					captionFadeTimer = null;
				}
				setCaptionsZoomHidden(true);
			});
			// zoomPanUpdate fires during pinch and after tap-zoom settles
			pswp.on('zoomPanUpdate', () => {
				const slide = pswp.currSlide;
				const isZoomed =
					slide !== undefined && slide.currZoomLevel > slide.zoomLevels.initial * 1.01;
				if (isZoomed) {
					// Covers pinch-to-zoom (beforeZoomTo doesn't fire for pinch)
					if (captionFadeTimer) {
						clearTimeout(captionFadeTimer);
						captionFadeTimer = null;
					}
					setCaptionsZoomHidden(true);
				} else if (!captionFadeTimer) {
					captionFadeTimer = setTimeout(() => {
						captionFadeTimer = null;
						setCaptionsZoomHidden(false);
					}, 0);
				}
				// Glue the caption to the photo during a vertical drag-to-close gesture.
				// PhotoSwipe moves only the slide's own container for that gesture, and the
				// caption lives in the item holder outside it — a horizontal swipe moves the
				// whole main-scroll container and takes the caption along for free, but a
				// vertical one leaves it behind. bounds.center.y is the at-rest pan position
				// (the same zero point PhotoSwipe measures the drag against). Applied even
				// while zoomed — the caption is invisible then, and the offset converges back
				// to zero on its own as the zoom-out animation re-centers the pan.
				const dragHolder = holders.find((h: ItemHolder) => h.slide === slide);
				const dragEl = dragHolder?.el.querySelector('.pswp-caption') as HTMLElement | null;
				if (dragEl) {
					const dy = slide ? slide.pan.y - slide.bounds.center.y : 0;
					dragEl.style.transform = dy ? `translate3d(0, ${dy}px, 0)` : '';
				}
			});
			// Reset caption opacity when navigating between slides
			pswp.on('change', () => {
				if (captionFadeTimer) {
					clearTimeout(captionFadeTimer);
					captionFadeTimer = null;
				}
				setCaptionsZoomHidden(false);
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
		const slugChanged = slug !== lastEffectSlug;
		const albumJustLoaded = album !== null && photos.length !== lastEffectPhotosLen;
		if (slugChanged || albumJustLoaded) {
			lastEffectSlug = slug;
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
			const timeouts = photos.map((photo: Photo, i: number) => {
				const src = `/albums/${slug}/${photo.src.grid}`;
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
			imageSrcs = photos.map((photo: Photo) => `/albums/${slug}/${photo.src.grid}`);
		}
	});

	function navigatePhotoCursor(currentIndex: number, direction: Direction) {
		const boxes = layout().boxes;
		if (!boxes || boxes.length === 0 || !container) return;
		const targetIndex = navigateCursor(boxes, currentIndex, direction);
		if (targetIndex !== null) {
			const target = container.querySelectorAll<HTMLElement>('.photo')[targetIndex];
			target?.focus();
			target?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
		}
	}

	function handlePhotoKeydown(e: KeyboardEvent, index: number) {
		if (!e.key.startsWith('Arrow')) return;
		e.preventDefault();
		navigatePhotoCursor(index, e.key.slice(5).toLowerCase() as Direction);
	}

	onMount(() => {
		syncSiteId(siteId, data.siteConfig?.keyId);

		const updateWidth = () => {
			if (container) {
				containerWidth = container.clientWidth;
			}
		};
		const handleKeydown = (e: KeyboardEvent) => {
			// Ignore ESC if lightbox is open or was just closed (same ESC keypress)
			if (e.key === 'Escape' && !lightboxOpen && Date.now() - lightboxClosedAt > 300) {
				goto(resolve('/'));
			}
		};
		updateWidth();
		layoutReady = true;

		if (albumEncrypted) {
			// Silently try stored passwords (fire and forget — resolves async).
			// On success, tryDecryptAlbum also handles auto-opening a permalink.
			tryDecryptAlbum();
		} else {
			// Open lightbox at the photo specified in the route (e.g. /albums/antarctica/15).
			// Skip the opening animation so it appears instantly rather than fading/zooming in.
			// invalidPhotoIndex (derived) handles the out-of-range case in the template.
			if (photoIndex !== null && invalidPhotoIndex === null) {
				openLightbox(photoIndex, false);
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
	title="{albumTitle} | {data.siteConfig.siteName}"
	description={plainDescription ||
		(album ? `${album.photos.length} photos from the '${albumTitle}' album` : albumTitle)}
	url="{data.siteConfig.siteUrl}/albums/{slug}"
	siteName={data.siteConfig.siteName}
	image={album ? `${data.siteConfig.siteUrl}/albums/${slug}/cover.jpg` : undefined}
/>

<!-- Header navigation: the configured album_nav links, or the built-in back link.
     album_nav hrefs come from albums.yaml, so they are plain strings and cannot go
     through resolve(), which only accepts SvelteKit's typed Pathname. Site-root-relative
     ones are prefixed with `base` instead, which is what resolve() would do for them;
     hence the eslint-disable on the anchor below. -->
{#snippet headerNav()}
	{#if albumNav.length > 0}
		<nav class="album-nav">
			<!-- eslint-disable svelte/no-navigation-without-resolve -- see note above -->
			{#each albumNav as link (link.id || link.href)}
				<a
					href={link.href.includes('://') || link.href.startsWith('mailto:')
						? link.href
						: base + link.href}
					id={link.id || undefined}
					target={link.newTab ? '_blank' : undefined}
					rel={link.newTab ? 'noopener' : undefined}>{link.label}</a
				>
			{/each}
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</nav>
	{:else}
		<a href={resolve('/')}>← Albums</a>
	{/if}
{/snippet}

{#if album}
	<main>
		<header>
			{@render headerNav()}
			<h1>{albumTitle}</h1>
			{#if description}
				<!-- eslint-disable-next-line svelte/no-at-html-tags -- albums.yaml description, HTML is intentional -->
				<p class="description">{@html description}</p>
			{/if}
			<p class="meta">
				{album.photos.length} photos{dateSpan ? ` · ${dateSpan}` : ''}
			</p>
		</header>

		{#if invalidPhotoIndex !== null}
			<div class="not-found">
				<p>
					Sorry, there is no photo No. {invalidPhotoIndex} in this album. Maybe we lost the negative?
				</p>
				<a href={resolve(`/albums/${slug}`)} class="back-link"
					><ArrowLeft size={16} aria-hidden="true" />Back to '{album.title}'</a
				>
			</div>
		{/if}

		<div
			class="gallery"
			bind:this={container}
			style="height: {Math.ceil(layout().containerHeight)}px;"
			class:layout-ready={layoutReady}
		>
			{#each album.photos as photo, i (photo.id)}
				{@const box = layout().boxes[i]}
				<button
					class="photo"
					data-index={i}
					aria-label={photoLabel(photo)}
					style="
						position: absolute;
						left: {box.left}px;
						top: {box.top}px;
						width: {box.width}px;
						height: {box.height}px;
					"
					onclick={() => openLightbox(i)}
					onkeydown={(e) => handlePhotoKeydown(e, i)}
				>
					<!-- src starts empty; set in onMount (immediately or after delay in ?slow mode).
					     loading="lazy" is dropped in slow mode to avoid browser deferring
					     images that are in-viewport when the delayed src is finally assigned.
					     The `loaded` class drives the fade-in transition in CSS. -->
					<img
						src={imageSrcs[i]}
						alt={stripTags(photo.description) || photo.fileName}
						width={box.width}
						height={box.height}
						loading={slowMode ? undefined : 'lazy'}
						class:loaded={imageLoaded[i]}
						onload={() => {
							imageLoaded[i] = true;
						}}
					/>
					{#if photo.kind === 'video'}
						<!-- The <img> above is the poster frame, so a video tile needs no
						     special image handling: only this badge marks it as playable. -->
						<div class="video-badge" aria-hidden="true">
							<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
								<path d="M8 5v14l11-7z" />
							</svg>
							{#if photo.duration}<span>{formatDuration(photo.duration)}</span>{/if}
						</div>
					{/if}
					{#if photo.description}
						<!-- inert because this sits inside the tile's <button>: a caption <a> would
						     otherwise be interactive content nested in a button (invalid HTML) and
						     add a tab stop per captioned tile. The button's aria-label already
						     carries the caption text, so nothing is lost by hiding it from AT. -->
						<!-- eslint-disable-next-line svelte/no-at-html-tags -- photogen.txt caption, HTML is intentional -->
						<div class="photo-caption" inert>{@html photo.description}</div>
					{/if}
				</button>
			{/each}
		</div>

		<BackToTop />
	</main>
{:else}
	<main class="loading-page">
		{#if !albumEncrypted}
			<header>
				{@render headerNav()}
				<h1>{albumTitle}</h1>
			</header>
		{/if}
	</main>
{/if}

{#if browser && albumEncrypted && !album && !unlocking}
	<div class="fullscreen-overlay">
		<PasswordPrompt
			prefix="Album"
			name={albumTitle}
			hint={albumHint}
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

	/* Configured album_nav links. They inherit the `header a` styling above, so an
	   unstyled config looks like the default back link; custom CSS targets them by id. */
	.album-nav {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
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
		padding: 1rem 1rem;
		text-align: center;
		color: var(--text-muted);
	}

	.not-found p {
		margin: 0 0 1rem 0;
		font-size: 1.1rem;
	}

	.not-found a {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
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

	/* Explicit :focus (not :focus-visible) so the outline appears on iOS after closing
	   the lightbox via the X button. Tapping X is a touch interaction, which switches iOS
	   to pointer modality and suppresses the default :focus-visible outline even when focus
	   is set programmatically. */
	.photo:focus {
		outline: 2px solid var(--focus-color, #0066cc);
		outline-offset: 2px;
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
		line-height: 1.2;
		text-align: center;
		/* Even out line lengths when a caption wraps, rather than leaving a lone
		   trailing word. Ignored by browsers without support, and browsers stop
		   balancing past a handful of lines — both fall back to normal wrapping. */
		text-wrap: balance;
		opacity: 0;
		transform: translateY(4px);
		transition:
			opacity 0.25s ease,
			transform 0.25s ease;
		pointer-events: none;
	}

	/* Caption links. In the grid they are styled but not clickable: the container is
	   pointer-events: none and the whole tile is one click target that opens the lightbox,
	   which is where a caption is meant to be read (and where its links work). */
	.photo-caption :global(a) {
		color: inherit;
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.photo:hover .photo-caption,
	.photo:focus .photo-caption {
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

	/* Play badge marking a grid tile as a video. Sits top-left so it never collides with
	   the caption, which slides up from the bottom. Always visible, unlike the caption:
	   it is the only thing distinguishing a clip from a still. */
	.video-badge {
		position: absolute;
		top: 0.4rem;
		left: 0.4rem;
		display: flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.15rem 0.4rem 0.15rem 0.3rem;
		border-radius: 999px;
		background: rgba(0, 0, 0, 0.62);
		color: white;
		font-size: 0.72rem;
		font-variant-numeric: tabular-nums;
		line-height: 1;
		pointer-events: none;
	}

	.video-badge svg {
		display: block;
	}

	/* PhotoSwipe customizations for dark theme */
	:global(.pswp) {
		--pswp-bg: #000;
	}

	/* Fully opaque background - hide content underneath */
	:global(.pswp__bg) {
		opacity: 1 !important;
	}

	/* Shade behind the top-bar controls.
	   The counter, zoom, copy-link and close controls are white with no background of
	   their own, so they vanish against a bright photo (a pale sky is the usual culprit).
	   This mirrors the caption's gradient, inverted, for the same reason.
	   Safe on both counts that matter: the bar carries .pswp__hide-on-close, so the shade
	   fades in and out with the rest of the UI, and the bar is pointer-events: none, so it
	   never intercepts a click. Absolutely positioned, so it is not a flex item and cannot
	   disturb the bar's layout. */
	:global(.pswp__top-bar::before) {
		content: '';
		position: absolute;
		inset: 0 0 auto 0;
		height: 100px;
		background: linear-gradient(rgba(0, 0, 0, 0.75), transparent);
		pointer-events: none;
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
	:global(.pswp__button--copy-link) {
		color: white;
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
		overflow: visible;
	}

	:global(.pswp__button--copy-link.copied) {
		opacity: 1;
		color: #6ddb6d;
	}

	:global(.pswp__button--copy-link.copied::after) {
		content: 'Link copied.';
		position: absolute;
		top: 40px;
		left: 50%;
		transform: translateX(-50%);
		background: rgba(0, 0, 0, 0.75);
		color: white;
		font-size: 12px;
		font-style: italic;
		white-space: nowrap;
		padding: 3px 8px;
		border-radius: 4px;
		pointer-events: none;
	}

	/* Lightbox caption — bottom/left/right set dynamically in JS to match the photo's
	   displayed box. The 0 values here are the pre-measurement fallback. */
	:global(.pswp-caption) {
		position: absolute;
		left: 0;
		right: 0;
		padding: 1.5rem 1rem 0.75rem;
		background: linear-gradient(transparent, rgba(0, 0, 0, 0.75));
		color: white;
		font-size: 0.9rem;
		line-height: 1.2;
		text-align: center;
		/* See .photo-caption — evens out wrapped lines instead of orphaning a word. */
		text-wrap: balance;
		pointer-events: none;
		z-index: 10;
		transition: opacity 0.3s ease;
	}

	/* Lightbox caption links are the one clickable thing in a caption, so they opt back in
	   to pointer events that the container above turns off. The container keeps
	   pointer-events: none so swipe and drag-to-close still work over the caption's text. */
	:global(.pswp-caption a) {
		pointer-events: auto;
		color: inherit;
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	:global(.pswp-caption a:hover),
	:global(.pswp-caption a:focus-visible) {
		text-decoration-thickness: 2px;
	}

	/* Video caption. The box reaches the bottom of the media like the photo rule above, but
	   its lower ~60px sit behind the browser's native control bar (padding-bottom is set in
	   JS from VIDEO_CONTROLS_HEIGHT), so the gradient has to keep darkening all the way down
	   rather than peaking at the text. The browser paints its own shade behind those controls
	   on top of this one; both run the same direction, so the two read as a single ramp
	   instead of the two stacked bands you get from a gradient that stops above the bar.
	   Peaks lower than the photo rule's 0.75 for the same reason: the two are additive down
	   there, and matching 0.75 makes the control area markedly darker than the caption. */
	:global(.pswp-caption--video) {
		padding-top: 3rem;
		background: linear-gradient(
			to bottom,
			rgba(0, 0, 0, 0) 0%,
			rgba(0, 0, 0, 0.45) 55%,
			rgba(0, 0, 0, 0.6) 100%
		);
	}

	/* Larger lightbox caption on desktop, where there's room for it */
	@media (min-width: 769px) {
		:global(.pswp-caption) {
			font-size: 1.2rem;
		}
	}

	/* Lightbox video. PhotoSwipe sets width/height inline on the wrapper from the slide's
	   zoom level, exactly as it does for an image, and a video's w/h in the data source
	   are its poster's — so the caption geometry in updateAll() needs no special case for
	   video slides. The video then fills the wrapper. */
	:global(.pswp-video-wrap) {
		display: flex;
		align-items: center;
		justify-content: center;
		background: #000;
	}

	:global(.pswp-video) {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: contain;
	}

	/* No rule is needed to hide PhotoSwipe's msrc placeholder on video slides: calling
	   content.onLoaded() in the contentLoad handler drops it the same way it does for a
	   photo. See the comment there before adding one back. */
</style>
