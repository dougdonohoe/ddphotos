import { describe, it, expect } from 'vitest';
import { albumFileUrl } from '$lib/albumUrl';

describe('albumFileUrl', () => {
	it('appends the version as ?v=', () => {
		expect(albumFileUrl('antarctica/grid/a.webp', '1a2b3c4d')).toBe(
			'/albums/antarctica/grid/a.webp?v=1a2b3c4d'
		);
	});

	it('returns the plain path without a version', () => {
		expect(albumFileUrl('antarctica/grid/a.webp')).toBe('/albums/antarctica/grid/a.webp');
		expect(albumFileUrl('antarctica/grid/a.webp', '')).toBe('/albums/antarctica/grid/a.webp');
	});

	it('encodes the version', () => {
		expect(albumFileUrl('a/b.webp', 'x&y')).toBe('/albums/a/b.webp?v=x%26y');
	});
});
