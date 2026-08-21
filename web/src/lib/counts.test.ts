import { describe, it, expect } from 'vitest';
import { mediaCountText, albumMetaText } from '$lib/counts';

describe('mediaCountText', () => {
	// Every rule the meta line follows, including the singular/plural boundary on each
	// half independently: an album can hold one video and many photos, or the reverse.
	const cases: [photos: number, videos: number, expected: string][] = [
		// No video: the wording albums had before video existed, unchanged.
		[0, 0, '0 photos'],
		[1, 0, '1 photo'],
		[2, 0, '2 photos'],
		[23, 0, '23 photos'],
		// Video only: the photo half disappears rather than showing "0 photos".
		[0, 1, '1 video'],
		[0, 4, '4 videos'],
		// Both: each half is pluralized on its own count.
		[1, 1, '1 photo · 1 video'],
		[1, 2, '1 photo · 2 videos'],
		[20, 1, '20 photos · 1 video'],
		[20, 3, '20 photos · 3 videos']
	];

	for (const [photos, videos, expected] of cases) {
		it(`${photos} photos and ${videos} videos reads "${expected}"`, () => {
			expect(mediaCountText(photos, videos)).toBe(expected);
		});
	}
});

describe('albumMetaText', () => {
	it('appends the date span after the counts', () => {
		expect(albumMetaText(20, 1, 'Dec 2004 - Jan 2005')).toBe(
			'20 photos · 1 video · Dec 2004 - Jan 2005'
		);
	});

	it('appends nothing when the album has no dated media', () => {
		// An album whose photos carry no EXIF date gets an empty dateSpan from photogen,
		// and albums.json omits it entirely for an empty album, so both arrive here.
		expect(albumMetaText(3, 0, '')).toBe('3 photos');
		expect(albumMetaText(3, 0, undefined)).toBe('3 photos');
		expect(albumMetaText(0, 0, undefined)).toBe('0 photos');
	});

	it('keeps the separator out of a single-kind album', () => {
		expect(albumMetaText(0, 2, 'Apr 2024')).toBe('2 videos · Apr 2024');
	});
});
