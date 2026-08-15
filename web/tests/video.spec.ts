import { test, expect } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockAlbumIfNeeded,
	findVideo,
	type FoundVideo
} from './helpers';

const pw = loadPasswords();

// Discovered once: video tests run against whatever the site actually publishes rather
// than a hardcoded album, matching how the rest of the suite handles optional content.
let video: FoundVideo | null = null;
test.beforeAll(async ({ request }) => {
	video = await findVideo(request);
});

/** Opens the album containing the video and returns its grid tile. */
async function openAlbum(page: import('@playwright/test').Page, v: FoundVideo) {
	await page.goto(`/albums/${v.slug}`);
	await unlockAlbumIfNeeded(page, v.slug, pw);
	await waitForHydration(page);
	return page.locator('.photo').nth(v.index);
}

/**
 * The slide currently on screen.
 *
 * PhotoSwipe keeps three slide holders alive (previous, current, next) and preloads their
 * content, so a bare `.pswp-video` locator matches neighbouring slides too and trips
 * Playwright's strict mode. PhotoSwipe marks the visible one with aria-hidden="false".
 */
function currentSlide(page: import('@playwright/test').Page) {
	return page.locator('.pswp__item[aria-hidden="false"]');
}

function currentVideo(page: import('@playwright/test').Page) {
	return currentSlide(page).locator('.pswp-video');
}

test('the mp4 is served with a video content type and byte ranges', async ({ request }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;

	// Both matter and neither is automatic in dev: without a video/* content type the
	// browser will not treat the response as media, and without range support seeking
	// does nothing and Safari refuses to play the file at all.
	const full = await request.get(v.videoUrl);
	expect(full.ok()).toBe(true);
	expect(full.headers()['content-type']).toBe('video/mp4');
	expect(full.headers()['accept-ranges']).toBe('bytes');

	const ranged = await request.get(v.videoUrl, { headers: { Range: 'bytes=0-99' } });
	expect(ranged.status()).toBe(206);
	expect(ranged.headers()['content-range']).toMatch(/^bytes 0-99\/\d+$/);
	expect((await ranged.body()).length).toBe(100);
});

test('the transcoded mp4 starts with its moov atom so playback can begin early', async ({
	request
}) => {
	test.skip(!video, 'no video published on this site');

	// -movflags +faststart puts the index at the front. Without it a browser must download
	// the whole file before it can play a single frame, which is the difference between a
	// clip that starts instantly and one that appears broken on a slow connection.
	const head = await (
		await request.get(video!.videoUrl, { headers: { Range: 'bytes=0-2047' } })
	).body();
	const text = head.toString('latin1');
	const moov = text.indexOf('moov');
	expect(moov).toBeGreaterThanOrEqual(0);
	const mdat = text.indexOf('mdat');
	if (mdat >= 0) expect(moov).toBeLessThan(mdat);
});

test('grid tile shows a play badge with the duration', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;
	const tile = await openAlbum(page, v);

	const badge = tile.locator('.video-badge');
	await expect(badge).toBeVisible();
	await expect(badge.locator('svg')).toBeVisible();
	if (v.duration >= 1) {
		await expect(badge).toHaveText(/\d+:\d{2}/);
	}

	// The tile still renders the poster as an ordinary image, which is what lets the
	// justified layout treat videos and stills identically.
	await expect(tile.locator('img')).toBeVisible();
});

test('grid tile is announced as a video to screen readers', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);

	// The badge is aria-hidden, so without this the tile is indistinguishable from a still.
	await expect(tile).toHaveAttribute('aria-label', /^Video:/);
});

test('clicking a video tile opens a playable video in the lightbox', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;
	const tile = await openAlbum(page, v);
	await tile.click();

	await expect(page.locator('.pswp')).toBeVisible();
	const player = currentVideo(page);
	await expect(player).toBeVisible();

	await expect(player).toHaveAttribute('src', v.videoUrl);
	await expect(player).toHaveAttribute('poster', v.posterUrl);
	await expect(player).toHaveAttribute('controls', '');
	// Muted by default so swiping an album never blares audio; the viewer can unmute.
	expect(await player.evaluate((el: HTMLVideoElement) => el.muted)).toBe(true);
	// Without playsinline, iOS Safari takes over the screen with its own player and
	// leaves PhotoSwipe's state out of sync when the viewer exits it.
	expect(await player.evaluate((el: HTMLVideoElement) => el.playsInline)).toBe(true);

	// Metadata loading proves the browser accepted the container and codec. A file the
	// browser cannot decode (HEVC in .mov, the untranscoded source) never reaches this.
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.readyState), { timeout: 10_000 })
		.toBeGreaterThan(0);
	expect(await player.evaluate((el: HTMLVideoElement) => el.videoWidth)).toBeGreaterThan(0);
});

test('video actually plays', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();

	const player = currentVideo(page);
	await expect(player).toBeVisible();
	await player.evaluate((el: HTMLVideoElement) => el.play());

	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.currentTime), { timeout: 10_000 })
		.toBeGreaterThan(0);
	expect(await player.evaluate((el: HTMLVideoElement) => el.paused)).toBe(false);
});

test('loading spinner does not linger on a video slide', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();
	await expect(currentVideo(page)).toBeVisible();

	// PhotoSwipe keeps custom content in its LOADING state unless contentLoad calls
	// content.onLoaded(), and shows the spinner once preloaderDelay (2s) elapses. It then
	// never clears, because nothing ever completes. Waiting past that delay is the whole
	// point of this test: a shorter wait passes even when the bug is present.
	await page.waitForTimeout(2600);
	await expect(page.locator('.pswp__preloader--active')).toHaveCount(0);
});

test('native controls are reachable by real pointer events', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();

	const player = currentVideo(page);
	await expect(player).toBeVisible();

	// Regression guard. PhotoSwipe leaves its msrc placeholder <img> on top of a slide
	// whose content load we preventDefault, and it swallowed every click aimed at the
	// control bar: the controls rendered but play, scrub and unmute all did nothing.
	// Driving play() from JS does not catch this, so hover and click for real.
	await expect(currentSlide(page).locator('.pswp__img--placeholder')).toBeHidden();
	await player.hover({ timeout: 5000 });

	// Chrome's built-in controls live in a closed shadow root, so they cannot be targeted
	// directly. Clicking the video's lower centre lands on the control bar; asserting the
	// element under that point belongs to the video proves nothing is covering it.
	const box = await player.boundingBox();
	expect(box).not.toBeNull();
	const onTop = await page.evaluate(
		([x, y]) => {
			const el = document.elementFromPoint(x, y);
			return el?.tagName ?? 'NONE';
		},
		[box!.x + box!.width / 2, box!.y + box!.height - 12]
	);
	expect(onTop).toBe('VIDEO');
});

test('space toggles play and pause', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();

	const player = currentVideo(page);
	await expect(player).toBeVisible();
	expect(await player.evaluate((el: HTMLVideoElement) => el.paused)).toBe(true);

	await page.keyboard.press(' ');
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(false);

	await page.keyboard.press(' ');
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(true);
});

test('space still toggles once when the video itself has focus', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();

	const player = currentVideo(page);
	await expect(player).toBeVisible();

	// With focus on the video the browser toggles playback itself, on keyup. Our handler
	// stands aside in that case; handling it too would play on keydown and pause on keyup,
	// cancelling out and making the key look dead. Either way the user gets exactly one
	// toggle, which is what this asserts.
	await player.evaluate((el: HTMLVideoElement) => el.focus());
	await page.keyboard.press(' ');
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(false);
});

test('space is inert on a photo slide', async ({ page, request }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;

	const resp = await request.get(`/albums/${v.slug}/index.json`);
	const index: { photos: { kind?: string }[] } = await resp.json();
	const stillIndex = index.photos.findIndex((p) => p.kind !== 'video');
	test.skip(stillIndex < 0, 'album is all video');

	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));

	await page.goto(`/albums/${v.slug}`);
	await unlockAlbumIfNeeded(page, v.slug, pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(stillIndex).click();
	await expect(page.locator('.pswp')).toBeVisible();

	await page.keyboard.press(' ');
	await page.waitForTimeout(300);

	// The video handler must not throw when there is no video on the slide, and must not
	// disturb the lightbox.
	expect(errors).toEqual([]);
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(currentSlide(page).locator('.pswp-video')).toHaveCount(0);

	// Note: space still scrolls the page behind the lightbox on a photo slide. That is
	// pre-existing (PhotoSwipe does not bind space and never has), and deliberately left
	// alone here rather than silently changing photo behaviour as part of adding video.
});

test('zoom is disabled on a video slide but still available on photos', async ({
	page,
	request
}) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;

	const resp = await request.get(`/albums/${v.slug}/index.json`);
	const index: { photos: { kind?: string }[] } = await resp.json();
	const stillIndex = index.photos.findIndex((p) => p.kind !== 'video');

	const tile = await openAlbum(page, v);
	await tile.click();
	await expect(currentVideo(page)).toBeVisible();

	// Deliberate, and a consequence of the data source's type: 'video'. PhotoSwipe gates
	// zoom on Content.isZoomable(), which is image-content only, so the button, double-tap,
	// pinch and the z key are all inert here. Zooming would scale the native control bar
	// along with the picture and push play/scrub off screen.
	await expect(page.locator('.pswp')).not.toHaveClass(/pswp--zoom-allowed/);
	await expect(page.locator('.pswp__button--zoom')).toBeHidden();

	// The z key must not zoom, and must not disturb the slide either.
	await page.keyboard.press('z');
	await page.waitForTimeout(400);
	await expect(page.locator('.pswp')).not.toHaveClass(/pswp--zoomed-in/);
	await expect(currentVideo(page)).toBeVisible();

	// Photos in the same album must keep zoom, proving this is scoped to video.
	test.skip(stillIndex < 0, 'album is all video');
	await page.keyboard.press('Escape');
	await page.goto(`/albums/${v.slug}`);
	await unlockAlbumIfNeeded(page, v.slug, pw);
	await waitForHydration(page);
	await page.locator('.photo').nth(stillIndex).click();
	await expect(page.locator('.pswp')).toBeVisible();
	await expect(page.locator('.pswp')).toHaveClass(/pswp--zoom-allowed/);
});

test('playing video pauses when swiped away', async ({ page, request }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;

	const resp = await request.get(`/albums/${v.slug}/index.json`);
	const index: { photos: unknown[] } = await resp.json();
	test.skip(index.photos.length < 2, 'album has nothing to swipe to');

	const tile = await openAlbum(page, v);
	await tile.click();

	// Keyed to the src rather than the current slide: after the swipe this element is no
	// longer the visible one, which is exactly the state under test.
	const player = page.locator(`.pswp-video[src="${v.videoUrl}"]`);
	await expect(player).toBeVisible();
	await player.evaluate((el: HTMLVideoElement) => el.play());
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(false);

	// Without the contentDeactivate hook the clip keeps playing off-screen, and once the
	// viewer has unmuted it keeps making noise from a slide they can no longer see.
	await page.locator('.pswp__button--arrow--next').click();
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(true);
});

test('closing the lightbox pauses the video', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const tile = await openAlbum(page, video!);
	await tile.click();

	const player = currentVideo(page);
	await expect(player).toBeVisible();
	await player.evaluate((el: HTMLVideoElement) => el.play());
	await expect
		.poll(() => player.evaluate((el: HTMLVideoElement) => el.paused), { timeout: 5000 })
		.toBe(false);

	const paused = player.evaluate((el: HTMLVideoElement) => {
		return new Promise<boolean>((resolve) => {
			el.addEventListener('pause', () => resolve(true), { once: true });
			setTimeout(() => resolve(el.paused), 4000);
		});
	});
	await page.keyboard.press('Escape');
	expect(await paused).toBe(true);
});

test('video slide keeps its caption positioned over the video', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;
	const tile = await openAlbum(page, v);

	// Only meaningful when this particular video has a caption.
	const hasCaption = (await tile.locator('.photo-caption').count()) > 0;
	test.skip(!hasCaption, 'video has no caption');

	await tile.click();
	const player = currentVideo(page);
	await expect(player).toBeVisible();

	// The caption geometry is computed from the slide's w/h, which for a video are its
	// poster's. Custom content bypasses PhotoSwipe's own image layout, so this asserts the
	// caption still lands on the video rather than somewhere in the black surround.
	const caption = currentSlide(page).locator('.pswp-caption').filter({ hasText: /\S/ });
	await expect(caption).toBeVisible();

	const capBox = await caption.boundingBox();
	const vidBox = await player.boundingBox();
	expect(capBox).not.toBeNull();
	expect(vidBox).not.toBeNull();
	expect(capBox!.x + capBox!.width).toBeGreaterThan(vidBox!.x);
	expect(capBox!.x).toBeLessThan(vidBox!.x + vidBox!.width);
	expect(capBox!.y).toBeLessThan(vidBox!.y + vidBox!.height + 1);

	// The box runs to the bottom of the video so its gradient continues into the shade the
	// browser draws behind the controls, reading as one ramp rather than two stacked bands.
	await expect(caption).toHaveClass(/pswp-caption--video/);
	const capBottom = capBox!.y + capBox!.height;
	const vidBottom = vidBox!.y + vidBox!.height;
	expect(Math.abs(capBottom - vidBottom)).toBeLessThan(2);

	// What keeps the text off the play button and scrubber is bottom padding, not a lifted
	// box. VIDEO_CONTROLS_HEIGHT is 48 in the page; assert a floor under that so the test
	// pins the behaviour rather than the exact constant.
	const padding = await caption.evaluate((el) => parseFloat(getComputedStyle(el).paddingBottom));
	expect(padding).toBeGreaterThan(40);
});

test('video caption hides while playing and returns on pause', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;
	const tile = await openAlbum(page, v);
	test.skip((await tile.locator('.photo-caption').count()) === 0, 'video has no caption');

	await tile.click();
	const player = currentVideo(page);
	await expect(player).toBeVisible();
	const caption = currentSlide(page).locator('.pswp-caption').filter({ hasText: /\S/ });

	// The caption sits above the browser's control bar, which is only on screen while the
	// video is paused or ended — so the caption tracks playback rather than the bar itself.
	// Without this it would be left floating mid-frame once the controls auto-hide.
	// toHaveCSS retries, which rides out the 0.3s opacity transition.
	await expect(caption).toHaveCSS('opacity', '1');

	await player.evaluate((el: HTMLVideoElement) => el.play());
	await expect(caption).toHaveCSS('opacity', '0');

	await player.evaluate((el: HTMLVideoElement) => el.pause());
	await expect(caption).toHaveCSS('opacity', '1');
});

test('video caption returns when the clip ends', async ({ page }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;
	const tile = await openAlbum(page, v);
	test.skip((await tile.locator('.photo-caption').count()) === 0, 'video has no caption');

	await tile.click();
	const player = currentVideo(page);
	await expect(player).toBeVisible();
	const caption = currentSlide(page).locator('.pswp-caption').filter({ hasText: /\S/ });

	// Reaching the end puts the controls back up, so the caption has to come back with them —
	// a separate path from an explicit pause, and one a viewer hits on every clip they watch
	// through. Seek to shortly before the end rather than playing the whole thing, leaving
	// enough runway to observe the caption actually hide first so the final assertion is not
	// vacuous.
	await player.evaluate(async (el: HTMLVideoElement) => {
		el.currentTime = Math.max(0, el.duration - 1.5);
		await el.play();
	});
	await expect(caption).toHaveCSS('opacity', '0');

	await expect.poll(() => player.evaluate((el: HTMLVideoElement) => el.ended)).toBe(true);
	await expect(caption).toHaveCSS('opacity', '1');
});

test('stills in the same album are unaffected', async ({ page, request }) => {
	test.skip(!video, 'no video published on this site');
	const v = video!;

	const resp = await request.get(`/albums/${v.slug}/index.json`);
	const index: { photos: { kind?: string }[] } = await resp.json();
	const stillIndex = index.photos.findIndex((p) => p.kind !== 'video');
	test.skip(stillIndex < 0, 'album is all video');

	await page.goto(`/albums/${v.slug}`);
	await unlockAlbumIfNeeded(page, v.slug, pw);
	await waitForHydration(page);

	const still = page.locator('.photo').nth(stillIndex);
	await expect(still.locator('.video-badge')).toHaveCount(0);

	await still.click();
	await expect(page.locator('.pswp')).toBeVisible();
	// Scoped to the current slide: the adjacent video slide is preloaded and its <video>
	// is in the DOM, just not on screen.
	await expect(currentSlide(page).locator('.pswp-video')).toHaveCount(0);
	await expect(currentSlide(page).locator('.pswp__img').first()).toBeVisible();

	// A photo caption keeps the plain gradient: it has no control bar to run past, so its
	// text sits at the bottom of the box rather than being padded up off one.
	const caption = currentSlide(page).locator('.pswp-caption').filter({ hasText: /\S/ });
	if (await caption.count()) {
		await expect(caption).not.toHaveClass(/pswp-caption--video/);
		// A photo has no playback state to hide behind, so its caption is unconditionally
		// visible. Guards the shared opacity path against a video-only rule leaking out.
		await expect(caption).toHaveCSS('opacity', '1');
	}
});
