// The lightbox slideshow: how long each photo stays up, and which slide comes next.
//
// The delay is the viewer's preference, kept in localStorage the same way as videoAudio's
// mute choice: not scoped to siteId, kept by clearStoredKeys through logout and build
// changes, and backed by a module-level copy for browsers where storage throws. Read
// lazily on first use rather than at import, which keeps this module free of
// $app/environment and so testable under plain Vitest.
export const SLIDESHOW_DELAY_KEY = 'ddp_slideshow_delay';

// Seconds per photo offered in the lightbox's duration menu.
export const SLIDESHOW_PRESETS = [1, 2, 3, 5, 8, 10, 15] as const;
const DEFAULT_DELAY = 3;

function isPreset(value: unknown): value is number {
	return typeof value === 'number' && (SLIDESHOW_PRESETS as readonly number[]).includes(value);
}

let delay: number | null = null;

function load(): number {
	try {
		const parsed = Number(localStorage.getItem(SLIDESHOW_DELAY_KEY));
		if (isPreset(parsed)) return parsed;
	} catch {
		// Storage unavailable (or not a browser): fall through to the default.
	}
	return DEFAULT_DELAY;
}

// Seconds each photo is shown for.
export function getSlideshowDelay(): number {
	delay ??= load();
	return delay;
}

// Records a new delay. Anything that is not one of the presets is ignored, so a stale or
// hand-edited value can never produce a zero or runaway interval.
export function setSlideshowDelay(seconds: number): void {
	if (!isPreset(seconds)) return;
	delay = seconds;
	try {
		localStorage.setItem(SLIDESHOW_DELAY_KEY, String(seconds));
	} catch {
		// Storage unavailable: the module-level copy still carries it for this page.
	}
}

// The index the slideshow moves to after `current`: the next slide that is not a video,
// wrapping past the end back to the start. Returns null when there is nowhere else to go,
// meaning the album holds no photo other than the current slide.
export function nextSlideshowIndex(isVideo: readonly boolean[], current: number): number | null {
	const n = isVideo.length;
	for (let step = 1; step < n; step++) {
		const i = (current + step) % n;
		if (!isVideo[i]) return i;
	}
	return null;
}
