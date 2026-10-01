import { test, expect, type Page } from '@playwright/test';
import {
	waitForHydration,
	loadPasswords,
	unlockSiteIfNeeded,
	unlockAlbumIfNeeded,
	slugFromCard,
	findVideo,
	type FoundVideo
} from './helpers';
import { nextSlideshowIndex } from '../src/lib/slideshow';

// Lightbox slideshow: the play button beside zoom, its seconds-per-photo menu, and the
// timer behind them.
//
// Runs on Playwright's fake clock so a 5-15s interval costs nothing. Two consequences
// shape the helpers below. First, the clock fakes requestAnimationFrame too, so
// PhotoSwipe's opening animation only progresses when a test runs the clock. Second, the
// countdown starts when the slide's full-size image has loaded, which happens on real
// time; nextSlideIndexAfter therefore steps the clock in small increments until the slide
// changes, rather than jumping by exactly one interval and racing that load.

const pw = loadPasswords();

const ALBUM_URL = /\/albums\/[^/]+\/(\d+)$/;

/** Slug of the first album on the home page: content-driven, like the rest of the suite. */
async function firstAlbumSlug(page: Page): Promise<string> {
	await page.goto('/');
	await unlockSiteIfNeeded(page, pw);
	return slugFromCard(page.locator('.album-card').first());
}

/** Opens `slug` and returns which of its tiles are videos, read off the grid's play badges. */
async function openAlbum(page: Page, slug: string): Promise<boolean[]> {
	await page.goto(`/albums/${slug}`);
	await unlockAlbumIfNeeded(page, slug, pw);
	await waitForHydration(page);
	return page
		.locator('.photo')
		.evaluateAll((tiles) => tiles.map((t) => t.querySelector('.video-badge') !== null));
}

async function openLightboxAt(page: Page, index: number) {
	await page.locator('.photo').nth(index).click();
	await page.clock.runFor(1000); // opening animation
	await expect(page.locator('.pswp--open')).toBeVisible();
}

/** 0-based index of the slide on screen, from the permalink the lightbox keeps current. */
async function slideIndex(page: Page): Promise<number> {
	const match = new URL(page.url()).pathname.match(ALBUM_URL);
	return match ? Number(match[1]) - 1 : -1;
}

/** Runs the clock in small steps until the slide changes, returning the new index. */
async function nextSlideIndexAfter(page: Page, from: number): Promise<number> {
	await expect
		.poll(
			async () => {
				await page.clock.runFor(500);
				return slideIndex(page);
			},
			{ timeout: 15_000 }
		)
		.not.toBe(from);
	return slideIndex(page);
}

const controls = (page: Page) => page.locator('.pswp__slideshow');
const playButton = (page: Page) => page.locator('.pswp__button--slideshow');
const caret = (page: Page) => page.locator('.pswp__button--slideshow-menu');
const menu = (page: Page) => page.locator('.pswp__slideshow-menu');
const countdown = (page: Page) => page.locator('.pswp__slideshow-countdown');
const countdownFill = (page: Page) => page.locator('.pswp__slideshow-countdown-fill');

let slug = '';
let isVideo: boolean[] = [];

test.beforeEach(async ({ page }) => {
	await page.clock.install();
	slug = await firstAlbumSlug(page);
	isVideo = await openAlbum(page, slug);
	test.skip(isVideo.filter((v) => !v).length < 2, 'first album has fewer than two photos');
});

/** Index of the last photo in the album, so the next step has to wrap. */
const lastPhoto = () => isVideo.lastIndexOf(false);

test('play advances through the album and loops past the last photo', async ({ page }) => {
	const start = lastPhoto();
	await openLightboxAt(page, start);

	await expect(playButton(page)).toHaveAttribute('aria-label', 'Start slideshow');
	await playButton(page).click();
	await expect(playButton(page)).toHaveAttribute('aria-label', 'Pause slideshow');

	const first = await nextSlideIndexAfter(page, start);
	expect(first).toBe(nextSlideshowIndex(isVideo, start));
	const second = await nextSlideIndexAfter(page, first);
	expect(second).toBe(nextSlideshowIndex(isVideo, first));
});

test('pause stops the slideshow', async ({ page }) => {
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await playButton(page).click();
	await playButton(page).click();
	await expect(controls(page)).not.toHaveClass(/playing/);

	await page.clock.runFor(30_000);
	expect(await slideIndex(page)).toBe(start);
});

test('the seconds menu sets the pace and is remembered', async ({ page }) => {
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);

	await caret(page).click();
	await expect(menu(page)).toBeVisible();
	await expect(caret(page)).toHaveAttribute('aria-expanded', 'true');
	await menu(page).getByRole('menuitemradio', { name: '15 sec' }).click();
	await expect(menu(page)).toBeHidden();
	expect(await page.evaluate(() => localStorage.getItem('ddp_slideshow_delay'))).toBe('15');

	// Remembered across a reload, and shown as the checked choice.
	// The URL is the photo's permalink, so the reload reopens the lightbox on it. On an
	// encrypted album that waits on decrypting with the stored password, hence waiting for
	// the lightbox itself rather than for hydration alone.
	await page.reload();
	await expect(page.locator('.pswp--open')).toBeVisible();
	expect(await slideIndex(page)).toBe(start);
	await caret(page).click();
	await expect(menu(page).getByRole('menuitemradio', { name: '15 sec' })).toHaveAttribute(
		'aria-checked',
		'true'
	);
	await caret(page).click();

	// 10s in, a 15s interval has not run out. The countdown waits for the image to load,
	// which only ever pushes the advance later, so this cannot be a false pass.
	await playButton(page).click();
	await page.clock.runFor(10_000);
	expect(await slideIndex(page)).toBe(start);
	expect(await nextSlideIndexAfter(page, start)).toBe(nextSlideshowIndex(isVideo, start));
});

test('a manual step restarts the countdown', async ({ page }) => {
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await caret(page).click();
	await menu(page).getByRole('menuitemradio', { name: '15 sec' }).click();
	await playButton(page).click();

	await page.clock.runFor(10_000);
	await page.keyboard.press('ArrowRight');
	const landed = await slideIndex(page);
	expect(landed).not.toBe(start);

	// Another 10s: past the original countdown's 15s, inside the restarted one.
	await page.clock.runFor(10_000);
	expect(await slideIndex(page)).toBe(landed);
	await expect(controls(page)).toHaveClass(/playing/);
});

test('Space starts and stops the slideshow on a photo slide', async ({ page }) => {
	await openLightboxAt(page, isVideo.indexOf(false));
	await page.keyboard.press('Space');
	await expect(controls(page)).toHaveClass(/playing/);
	await page.keyboard.press('Space');
	await expect(controls(page)).not.toHaveClass(/playing/);
});

test('Space starts the slideshow after picking seconds with the mouse', async ({ page }) => {
	// Picking leaves focus where Space acts on the slideshow, not back on the caret, where
	// it would only reopen the menu.
	await openLightboxAt(page, isVideo.indexOf(false));
	await caret(page).click();
	await menu(page).getByRole('menuitemradio', { name: '8 sec' }).click();
	await expect(caret(page)).not.toBeFocused();

	await page.keyboard.press('Space');
	await expect(menu(page)).toBeHidden();
	await expect(controls(page)).toHaveClass(/playing/);
});

test('Space starts the slideshow after picking seconds with the keyboard', async ({ page }) => {
	await openLightboxAt(page, isVideo.indexOf(false));
	await caret(page).focus();
	await page.keyboard.press('Enter');
	await expect(menu(page)).toBeVisible();
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await expect(menu(page)).toBeHidden();
	await expect(playButton(page)).toBeFocused();

	await page.keyboard.press('Space');
	await expect(menu(page)).toBeHidden();
	await expect(controls(page)).toHaveClass(/playing/);
});

test('the play and caret icons hold still on hover and focus', async ({ page }) => {
	// PhotoSwipe's own .pswp__button:hover/:active/:focus rule zeroes padding, so any
	// offset these icons get from padding vanishes the moment the pointer arrives.
	await openLightboxAt(page, isVideo.indexOf(false));
	for (const button of [playButton(page), caret(page)]) {
		const icon = button.locator('svg');
		const before = await icon.boundingBox();
		await button.hover();
		expect(await icon.boundingBox()).toEqual(before);
		await button.focus();
		expect(await icon.boundingBox()).toEqual(before);
		await page.mouse.move(0, 400);
	}
});

test('a countdown pie beside the counter sweeps over each interval', async ({ page }) => {
	// The sweep is a CSS transition, which runs on real time rather than the fake clock,
	// so this checks how it is set up rather than watching it fill.
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await expect(countdown(page)).toBeHidden();

	await caret(page).click();
	await menu(page).getByRole('menuitemradio', { name: '8 sec' }).click();
	await playButton(page).click();
	await expect(countdown(page)).toBeVisible();
	await expect(countdownFill(page)).toHaveCSS('transition-duration', '8s');

	// Right of the counter, left of the zoom button.
	const counterBox = (await page.locator('.pswp__counter').boundingBox())!;
	const pieBox = (await countdown(page).boundingBox())!;
	const zoomBox = (await page.locator('.pswp__button--zoom').boundingBox())!;
	expect(pieBox.x).toBeGreaterThanOrEqual(counterBox.x + counterBox.width);
	expect(pieBox.x + pieBox.width).toBeLessThan(zoomBox.x);

	// Each new photo starts a fresh sweep, which begins from an instant reset to empty.
	// Catch that reset by clearing the transition's end value and seeing it restored.
	await countdownFill(page).evaluate((el) => (el.style.strokeDashoffset = 'x'));
	await nextSlideIndexAfter(page, start);
	await expect(countdownFill(page)).toHaveAttribute('style', /stroke-dashoffset: 0/);

	await playButton(page).click();
	await expect(countdown(page)).toBeHidden();
});

test('the countdown holds while a pointer is down', async ({ page }) => {
	// A slide changing mid-swipe leaves PhotoSwipe's drag working from stale points, so
	// the timer waits for the finger or button to come back up. Pressed on the counter
	// so the press itself does nothing else.
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await playButton(page).click();
	const counter = (await page.locator('.pswp__counter').boundingBox())!;
	await page.mouse.move(counter.x + 5, counter.y + 5);
	await page.mouse.down();
	await page.clock.runFor(20_000);
	expect(await slideIndex(page)).toBe(start);

	await page.mouse.up();
	expect(await nextSlideIndexAfter(page, start)).toBe(nextSlideshowIndex(isVideo, start));
});

test('the countdown holds while the seconds menu is open', async ({ page }) => {
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await playButton(page).click();
	await caret(page).click();
	await page.clock.runFor(20_000);
	expect(await slideIndex(page)).toBe(start);

	await page.keyboard.press('Escape');
	expect(await nextSlideIndexAfter(page, start)).toBe(nextSlideshowIndex(isVideo, start));
});

test('the countdown holds while the tab is hidden', async ({ page }) => {
	const setHidden = (hidden: boolean) =>
		page.evaluate((hidden) => {
			Object.defineProperty(document, 'visibilityState', {
				configurable: true,
				get: () => (hidden ? 'hidden' : 'visible')
			});
			document.dispatchEvent(new Event('visibilitychange'));
		}, hidden);
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await playButton(page).click();
	await setHidden(true);
	await page.clock.runFor(20_000);
	expect(await slideIndex(page)).toBe(start);

	await setHidden(false);
	expect(await nextSlideIndexAfter(page, start)).toBe(nextSlideshowIndex(isVideo, start));
});

test('Space starts the slideshow after clicking an arrow', async ({ page }) => {
	// Guards against Space re-clicking the arrow. That does not happen today because
	// PhotoSwipe prevents the mousedown that would focus a clicked top-bar button, and
	// this test keeps it that way.
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await page.locator('.pswp__button--arrow--next').click();
	const landed = await slideIndex(page);
	expect(landed).not.toBe(start);
	test.skip(isVideo[landed], 'the arrow landed on a video slide');

	await page.keyboard.press('Space');
	await expect(controls(page)).toHaveClass(/playing/);
	expect(await slideIndex(page)).toBe(landed);
});

test('the slideshow will not start on a zoomed-in photo', async ({ page }) => {
	await openLightboxAt(page, isVideo.indexOf(false));
	test.skip(
		!(await page.locator('.pswp--zoom-allowed').count()),
		'photo is not zoomable at this viewport'
	);
	await page.keyboard.press('z');
	await page.clock.runFor(1000);
	await playButton(page).click();
	await expect(controls(page)).not.toHaveClass(/playing/);
	await page.locator('.pswp').focus();
	await page.keyboard.press('Space');
	await expect(controls(page)).not.toHaveClass(/playing/);
});

test('ArrowUp from outside the menu items goes to the last choice', async ({ page }) => {
	// Rare in practice (a mouse press inside the menu does not move focus, because
	// PhotoSwipe prevents the mousedown), so focus is moved off the items directly.
	await openLightboxAt(page, isVideo.indexOf(false));
	await caret(page).click();
	await page.locator('.pswp').focus();
	await expect(menu(page)).toBeVisible();
	await page.keyboard.press('ArrowUp');
	await expect(menu(page).getByRole('menuitemradio', { name: '15 sec' })).toBeFocused();
});

test('Escape closes the open menu, not the lightbox', async ({ page }) => {
	await openLightboxAt(page, isVideo.indexOf(false));
	await caret(page).click();
	await expect(menu(page)).toBeVisible();

	await page.keyboard.press('Escape');
	await expect(menu(page)).toBeHidden();
	await expect(page.locator('.pswp--open')).toBeVisible();
	await expect(caret(page)).toBeFocused();
});

test('zooming in stops the slideshow', async ({ page }) => {
	await openLightboxAt(page, isVideo.indexOf(false));
	test.skip(
		!(await page.locator('.pswp--zoom-allowed').count()),
		'photo is not zoomable at this viewport'
	);
	await playButton(page).click();
	await page.keyboard.press('z');
	await page.clock.runFor(1000);
	await expect(controls(page)).not.toHaveClass(/playing/);
});

test('closing the lightbox stops the slideshow', async ({ page }) => {
	const start = isVideo.indexOf(false);
	await openLightboxAt(page, start);
	await playButton(page).click();
	await page.keyboard.press('Escape');
	await page.clock.runFor(1000);
	await expect(page.locator('.pswp')).toHaveCount(0);

	await openLightboxAt(page, start);
	await expect(controls(page)).not.toHaveClass(/playing/);
});

test.describe('with video', () => {
	let video: FoundVideo | null = null;
	test.beforeAll(async ({ request }) => {
		video = await findVideo(request);
	});

	test('video slides are skipped', async ({ page }) => {
		test.skip(!video, 'no video published on this site');
		const v = video!;
		isVideo = await openAlbum(page, v.slug);
		test.skip(isVideo.filter((x) => !x).length < 2, 'video album has fewer than two photos');

		// Start on the last photo before the video, so the next step has to jump it.
		let start = v.index;
		do start = (start - 1 + isVideo.length) % isVideo.length;
		while (isVideo[start]);
		await openLightboxAt(page, start);
		await playButton(page).click();

		const landed = await nextSlideIndexAfter(page, start);
		expect(isVideo[landed]).toBe(false);
		expect(landed).toBe(nextSlideshowIndex(isVideo, start));
	});
});
