import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// The module caches the delay on first read, so each test imports a fresh copy.
async function freshModule() {
	vi.resetModules();
	return import('$lib/slideshow');
}

describe('nextSlideshowIndex', () => {
	const P = false; // photo
	const V = true; // video

	it('moves to the next photo', async () => {
		const { nextSlideshowIndex } = await freshModule();
		expect(nextSlideshowIndex([P, P, P], 0)).toBe(1);
	});

	it('wraps from the last photo to the first', async () => {
		const { nextSlideshowIndex } = await freshModule();
		expect(nextSlideshowIndex([P, P, P], 2)).toBe(0);
	});

	it('skips videos, including across the wrap', async () => {
		const { nextSlideshowIndex } = await freshModule();
		expect(nextSlideshowIndex([P, V, V, P], 0)).toBe(3);
		expect(nextSlideshowIndex([V, P, P, V], 2)).toBe(1);
	});

	it('moves on from a video slide the viewer navigated to', async () => {
		const { nextSlideshowIndex } = await freshModule();
		expect(nextSlideshowIndex([P, V, P], 1)).toBe(2);
	});

	it('returns null when there is no other photo', async () => {
		const { nextSlideshowIndex } = await freshModule();
		expect(nextSlideshowIndex([P], 0)).toBeNull();
		expect(nextSlideshowIndex([P, V, V], 0)).toBeNull();
		expect(nextSlideshowIndex([V, V], 0)).toBeNull();
		expect(nextSlideshowIndex([], 0)).toBeNull();
	});
});

describe('slideshow delay', () => {
	let store: Map<string, string>;

	beforeEach(() => {
		store = new Map();
		vi.stubGlobal('localStorage', {
			getItem: (k: string) => store.get(k) ?? null,
			setItem: (k: string, v: string) => void store.set(k, v)
		});
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('defaults to 3 seconds', async () => {
		const { getSlideshowDelay } = await freshModule();
		expect(getSlideshowDelay()).toBe(3);
	});

	it('reads a stored preset', async () => {
		for (const seconds of ['1', '2', '10']) {
			store.set('ddp_slideshow_delay', seconds);
			const { getSlideshowDelay } = await freshModule();
			expect(getSlideshowDelay()).toBe(Number(seconds));
		}
	});

	it('ignores a stored value that is not a preset', async () => {
		for (const bad of ['0', '7', '-3', 'abc', '']) {
			store.set('ddp_slideshow_delay', bad);
			const { getSlideshowDelay } = await freshModule();
			expect(getSlideshowDelay(), `stored "${bad}"`).toBe(3);
		}
	});

	it('saves a preset and ignores anything else', async () => {
		const { getSlideshowDelay, setSlideshowDelay } = await freshModule();
		setSlideshowDelay(8);
		expect(getSlideshowDelay()).toBe(8);
		expect(store.get('ddp_slideshow_delay')).toBe('8');
		setSlideshowDelay(0);
		expect(getSlideshowDelay()).toBe(8);
		expect(store.get('ddp_slideshow_delay')).toBe('8');
	});

	it('keeps working when storage throws', async () => {
		vi.stubGlobal('localStorage', {
			getItem: () => {
				throw new Error('denied');
			},
			setItem: () => {
				throw new Error('denied');
			}
		});
		const { getSlideshowDelay, setSlideshowDelay } = await freshModule();
		expect(getSlideshowDelay()).toBe(3);
		setSlideshowDelay(8);
		expect(getSlideshowDelay()).toBe(8);
	});
});
